package file

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/welllog/golt/config/driver"
	"github.com/welllog/golt/config/meta"
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
}

func New(c meta.Config) (driver.Driver, error) {
	fd := file{
		namespace2node: make(map[string]*fileNode, len(c.Configs)),
		filepath2node:  make(map[string]*fileNode, len(c.Configs)),
	}

	root := c.SourceAddr()
	watch := false

	for _, cfg := range c.Configs {
		path := filepath.Join(root, cfg.Path)
		nps := cfg.Namespaces()

		node, ok := fd.filepath2node[path]
		if !ok {
			node = &fileNode{
				dynamic: cfg.Dynamic,
			}

			err := node.Load(path)
			if err != nil {
				return nil, fmt.Errorf("load file %s failed: %w", path, err)
			}

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

func (f *file) watch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("new watcher failed: %w", err)
	}
	f.watcher = watcher

	for path, node := range f.filepath2node {
		if node.dynamic {
			err = watcher.Add(path)
			if err != nil {
				return fmt.Errorf("watcher add path failed: %s", err.Error())
			}
		}
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				fmt.Println("event: ", event.Name, " op: ", event.Op)

				if event.Op != fsnotify.Write && event.Op != fsnotify.Create && event.Op != fsnotify.Chmod {
					continue
				}

				node, ok := f.filepath2node[event.Name]
				if !ok {
					continue
				}

				f.mu.Lock()
				err := node.Load(event.Name)
				f.mu.Unlock()

				if err != nil {
					continue
				}

				f.mu.RLock()
				node.ExecuteHook()
				f.mu.RUnlock()

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Printf("watch err: %v", err)
			}
		}
	}()

	return nil
}
