package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
	"github.com/welllog/golib/mapz"
	"github.com/welllog/golib/setz"
	"github.com/welllog/golt/config/driver"
	"github.com/welllog/golt/config/meta"
	"github.com/welllog/golt/contract"
	"gopkg.in/yaml.v3"
)

var _ driver.Driver = (*file)(nil)

func init() {
	driver.RegisterDriver("file", New)
}

type file struct {
	watcher        *fsnotify.Watcher
	mu             sync.RWMutex
	namespace2node map[string]*fileNode
	filepath2node  map[string]*fileNode
	buf            map[string]*field
	ch             chan string
	quit           chan struct{}
	logger         contract.Logger
}

func New(c meta.Config, logger contract.Logger) (driver.Driver, error) {
	fd := file{
		namespace2node: make(map[string]*fileNode, len(c.Configs)),
		filepath2node:  make(map[string]*fileNode, len(c.Configs)),
		buf:            make(map[string]*field),
		logger:         logger,
		quit:           make(chan struct{}),
	}

	root := c.SourceAddr()
	watch := false

	for _, cfg := range c.Configs {
		path := filepath.Join(root, cfg.Path)
		nps := cfg.Namespaces()

		node, ok := fd.filepath2node[path]
		if !ok {
			err := fd.loadToBuf(path)
			if err != nil {
				return nil, fmt.Errorf("load file %s failed: %w", path, err)
			}

			node = &fileNode{}
			node.CacheFrom(fd.buf)
			clear(fd.buf)
			fd.filepath2node[path] = node
		}

		if cfg.Watch {
			node.watch = true
			watch = true
		}

		for _, np := range nps {
			fd.namespace2node[np] = node
		}
	}

	if len(fd.namespace2node) == 0 {
		return nil, errors.New("config rules is empty")
	}

	if watch {
		if err := fd.watch(); err != nil {
			fd.Close()
			return nil, err
		}
	}

	return &fd, nil
}

func (f *file) Namespaces() []string {
	nps := make([]string, 0, len(f.namespace2node))
	for np := range f.namespace2node {
		nps = append(nps, np)
	}
	return nps
}

func (f *file) OnKeyChange(namespace, key string, hook func([]byte) error) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	node, ok := f.namespace2node[namespace]
	if !ok {
		return false
	}

	return node.OnKeyChange(key, hook)
}

func (f *file) Get(ctx context.Context, namespace, key string) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	node, ok := f.namespace2node[namespace]
	if !ok {
		return nil, driver.ErrNotFound
	}

	b, ok := node.UnsafeGet(key)
	if !ok {
		return nil, driver.ErrNotFound
	}

	return b, nil
}

func (f *file) GetString(ctx context.Context, namespace, key string) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	node, ok := f.namespace2node[namespace]
	if !ok {
		return "", driver.ErrNotFound
	}

	value, ok := node.GetString(key)
	if !ok {
		return "", driver.ErrNotFound
	}

	return value, nil
}

func (f *file) Close() {
	select {
	case <-f.quit:
		return
	default:
		close(f.quit)
	}

	if f.watcher != nil {
		_ = f.watcher.Close()
	}
}

func (f *file) loadToBuf(path string) error {
	var fn func([]byte, any) error

	ext := filepath.Ext(path)
	switch ext {
	case ".json":
		fn = json.Unmarshal
	case ".yaml", ".yml":
		fn = yaml.Unmarshal
	case ".toml":
		fn = toml.Unmarshal
	default:
		return errors.New("file format only support json/yaml/toml, but got " + ext)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return fn(b, &f.buf)
}

func (f *file) watch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("new watcher failed: %w", err)
	}
	f.watcher = watcher
	f.ch = make(chan string)

	set := make(setz.Set[string], len(f.filepath2node))
	for path, node := range f.filepath2node {
		if node.watch {
			dir := filepath.Dir(path)
			if set.Add(dir) {
				f.logger.Debugf("watch path: %s", dir)
				err = watcher.Add(dir)
				if err != nil {
					return fmt.Errorf("watcher add path failed: %s", err.Error())
				}
			}
		}
	}

	go f.dedup()

	go f.listenAndRefresh()

	return nil
}

func (f *file) dedup() {
	// Wait 500ms for new events; each new event resets the timer.
	const waitFor = 500 * time.Millisecond
	path2timers := mapz.NewSafeKV[string, *time.Timer](5)
	eventFunc := func(name string) {
		path2timers.Delete(name)
		select {
		case <-f.quit:
		case f.ch <- name:
		}
	}

	for {
		select {
		case <-f.quit:
			return
		case err, ok := <-f.watcher.Errors:
			if !ok { // Channel was closed (i.e. Watcher.Close() was called).
				return
			}

			f.logger.Errorf("file watcher err: %v", err)
		case e, ok := <-f.watcher.Events:
			if !ok { // Channel was closed (i.e. Watcher.Close() was called).
				return
			}

			if !e.Has(fsnotify.Create) && !e.Has(fsnotify.Write) {
				continue
			}

			node, ok := f.filepath2node[e.Name]
			if !ok {
				continue
			}

			if !node.watch {
				continue
			}

			t, ok := path2timers.Get(e.Name)
			// No timer yet, so create one.
			if !ok {
				t = time.AfterFunc(waitFor, func() {
					eventFunc(e.Name)
				})
				path2timers.Set(e.Name, t)
			} else {
				// Reset the timer for this path, so it will start from 500ms again.
				t.Reset(waitFor)
			}
		}
	}
}

func (f *file) listenAndRefresh() {
	for {
		select {
		case <-f.quit:
			return
		case path := <-f.ch:
			node, ok := f.filepath2node[path]
			if !ok {
				continue
			}

			f.logger.Debugf("file %s changed", path)

			if err := f.loadToBuf(path); err != nil {
				f.logger.Errorf("reload file %s failed: %v", path, err)
				clear(f.buf)
				continue
			}

			f.mu.Lock()
			node.CacheFrom(f.buf)
			f.mu.Unlock()

			// hookFlag update not need lock, because it only set true here and set false in ExecuteHook
			f.mu.RLock()
			node.ExecuteHook(f.buf, f.logger)
			f.mu.RUnlock()

			clear(f.buf)
		}
	}
}
