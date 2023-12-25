package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/welllog/golt/config/driver"
	"github.com/welllog/golt/config/meta"
	"gopkg.in/yaml.v3"

	_ "github.com/welllog/golt/config/driver/file"
)

var ErrNotFound = driver.ErrNotFound

type Engine struct {
	ds map[string]driver.Driver
}

func EngineFromFile(file string) (*Engine, error) {
	var (
		cs []meta.Config
		fn func([]byte, any) error
	)

	ext := filepath.Ext(file)
	switch ext {
	case ".json":
		fn = json.Unmarshal
	case ".yaml":
		fn = yaml.Unmarshal
	default:
		return nil, errors.New("unsupported config file format")
	}

	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	if err := fn(b, &cs); err != nil {
		return nil, err
	}

	return newEngine(cs)
}

func newEngine(cs []meta.Config) (*Engine, error) {
	e := Engine{
		ds: make(map[string]driver.Driver, len(cs)*2),
	}

	for _, c := range cs {
		d, err := driver.New(c)
		if err != nil {
			e.Close()
			return nil, err
		}

		for _, v := range d.Namespaces() {
			_, ok := e.ds[v]
			if ok {
				d.Close()
				e.Close()
				return nil, errors.New("duplicate namespace:" + v)
			}
			e.ds[v] = d
		}
	}

	return &e, nil
}

func (e *Engine) RegisterHook(namespace, key string, hook func([]byte) error) bool {
	d, ok := e.ds[namespace]
	if !ok {
		return false
	}

	return d.RegisterHook(namespace, key, hook)
}

func (e *Engine) Get(namespace, key string) ([]byte, error) {
	d, ok := e.ds[namespace]
	if !ok {
		return nil, ErrNotFound
	}

	return d.Get(namespace, key)
}

func (e *Engine) UnsafeGet(namespace, key string) ([]byte, error) {
	d, ok := e.ds[namespace]
	if !ok {
		return nil, ErrNotFound
	}

	return d.Get(namespace, key)
}

func (e *Engine) Decode(namespace, key string, value any, unmarshalFunc func([]byte, any) error) error {
	d, ok := e.ds[namespace]
	if !ok {
		return ErrNotFound
	}

	return d.Decode(namespace, key, value, unmarshalFunc)
}

func (e *Engine) Close() {
	for _, v := range e.ds {
		v.Close()
	}
}
