package file

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/welllog/golib/dsz"
	"github.com/welllog/golib/mapz"
	"github.com/welllog/golt/config/driver"
	"github.com/welllog/golt/config/meta"
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
}

func New(c meta.Config) (driver.Driver, error) {
	fd := file{
		namespace2node: make(map[string]*fileNode, len(c.Configs)),
		filepath2node:  make(map[string]*fileNode, len(c.Configs)),
		buf:            make(map[string]*field),
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
			node.SetFields(fd.buf)
			fd.filepath2node[path] = node
		}

		if cfg.Dynamic {
			node.dynamic = true
			watch = true
		}

		for _, np := range nps {
			fd.namespace2node[np] = node
		}
	}

	if len(fd.namespace2node) == 0 {
		return nil, errors.New("config files is empty")
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

func (f *file) RegisterHook(namespace, key string, hook func([]byte) error) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	node, ok := f.namespace2node[namespace]
	if !ok {
		return false
	}

	return node.RegisterHook(key, hook)
}

func (f *file) Get(namespace, key string) ([]byte, error) {
	b, err := f.UnsafeGet(namespace, key)
	if err != nil {
		return nil, err
	}

	return append([]byte(nil), b...), nil
}

func (f *file) UnsafeGet(namespace, key string) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	node, ok := f.namespace2node[namespace]
	if !ok {
		return nil, driver.ErrNotFound
	}

	b, ok := node.Get(key)
	if !ok {
		return nil, driver.ErrNotFound
	}

	return b, nil
}

func (f *file) Decode(namespace, key string, value any, unmarshalFunc func([]byte, any) error) error {
	b, err := f.UnsafeGet(namespace, key)
	if err != nil {
		return err
	}

	return unmarshalFunc(b, value)
}

func (f *file) Close() {
	if f.watcher != nil {
		_ = f.watcher.Close()
	}
}

func (f *file) loadToBuf(path string) error {
	for k := range f.buf {
		delete(f.buf, k)
	}

	var fn func([]byte, any) error

	ext := filepath.Ext(path)
	switch ext {
	case ".json":
		fn = json.Unmarshal
	case ".yaml":
		fn = yaml.Unmarshal
	default:
		return errors.New("unsupported config file format: " + ext)
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

	set := make(dsz.Set[string], len(f.filepath2node))
	for path, node := range f.filepath2node {
		if node.dynamic {
			dir := filepath.Dir(path)
			if set.Add(dir) {
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
	var (
		// Wait 500ms for new events; each new event resets the timer.
		waitFor     = 500 * time.Millisecond
		path2timers = mapz.NewSafeKV[string, *time.Timer](5)
		eventFunc   = func(name string) {
			path2timers.Delete(name)
			f.ch <- name
		}
	)

	for {
		select {
		case err, ok := <-f.watcher.Errors:
			if !ok { // Channel was closed (i.e. Watcher.Close() was called).
				return
			}
			fmt.Printf("watch err: %v", err)
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

			if !node.dynamic {
				continue
			}

			t, ok := path2timers.Get(e.Name)
			// No timer yet, so create one.
			if !ok {
				t = time.AfterFunc(math.MaxInt64, func() { eventFunc(e.Name) })
				t.Stop()

				path2timers.Set(e.Name, t)
			}

			// Reset the timer for this path, so it will start from 500ms again.
			t.Reset(waitFor)
		}
	}
}

func (f *file) listenAndRefresh() {
	for {
		path := <-f.ch

		node, ok := f.filepath2node[path]
		if !ok {
			continue
		}

		if err := f.loadToBuf(path); err != nil {
			// TODO log
			continue
		}

		f.mu.Lock()
		node.SetFields(f.buf)
		f.mu.Unlock()

		f.mu.RLock()
		node.ExecuteHook()
		f.mu.RUnlock()
	}
}
