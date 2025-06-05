package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/welllog/golt/config/driver"
	"github.com/welllog/golt/config/meta"
	"github.com/welllog/golt/contract"
	"github.com/welllog/olog"
	"gopkg.in/yaml.v3"

	_ "github.com/welllog/golt/config/driver/etcd"
	_ "github.com/welllog/golt/config/driver/file"
)

var ErrNotFound = driver.ErrNotFound

type Configure struct {
	ds     map[string]driver.Driver
	logger contract.Logger
}

func FromFile(file string, logger contract.Logger) (*Configure, error) {
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

	if logger == nil {
		logger = olog.GetLogger()
	}
	return newConfigure(cs, logger)
}

func newConfigure(cs []meta.Config, logger contract.Logger) (*Configure, error) {
	e := Configure{
		ds:     make(map[string]driver.Driver, len(cs)*2),
		logger: logger,
	}

	for _, c := range cs {
		d, err := driver.New(c, logger)
		if err != nil {
			logger.Errorf("new driver failed: %s %s", c.SourceSchema(), c.SourceAddr())
			e.Close()
			return nil, err
		}

		for _, v := range d.Namespaces() {
			_, ok := e.ds[v]
			if ok {
				d.Close()
				e.Close()
				logger.Errorf("duplicate namespace: %s on %s %s", v, c.SourceSchema(), c.SourceAddr())
				return nil, errors.New("duplicate namespace:" + v)
			}
			e.ds[v] = d
		}
	}

	return &e, nil
}

func (e *Configure) OnKeyChange(namespace, key string, hook func([]byte) error) bool {
	d, ok := e.ds[namespace]
	if ok {
		ok = d.OnKeyChange(namespace, key, hook)
	}

	if !ok {
		e.logger.Warnf("OnKeyChange register failed: namespace=%s key=%s", namespace, key)
	}

	return ok
}

func (e *Configure) GetRaw(namespace, key string) ([]byte, error) {
	b, err := e.UnsafeGetRaw(namespace, key)
	if err != nil {
		return nil, err
	}

	return append([]byte(nil), b...), nil
}

func (e *Configure) UnsafeGetRaw(namespace, key string) ([]byte, error) {
	d, ok := e.ds[namespace]
	if !ok {
		return nil, ErrNotFound
	}

	return d.Get(namespace, key)
}

func (e *Configure) GetRawString(namespace, key string) (string, error) {
	d, ok := e.ds[namespace]
	if !ok {
		return "", ErrNotFound
	}

	return d.GetString(namespace, key)
}

func (e *Configure) String(namespace, key string) (string, error) {
	s, err := e.GetRawString(namespace, key)
	if err != nil {
		return "", err
	}

	return unquote(s), nil
}

func (e *Configure) Int64(namespace, key string) (int64, error) {
	s, err := e.String(namespace, key)
	if err != nil {
		return 0, err
	}

	return strconv.ParseInt(s, 10, 64)
}

func (e *Configure) Int(namespace, key string) (int, error) {
	s, err := e.String(namespace, key)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(s)
}

func (e *Configure) Float64(namespace, key string) (float64, error) {
	s, err := e.String(namespace, key)
	if err != nil {
		return 0, err
	}

	return strconv.ParseFloat(s, 64)
}

func (e *Configure) Bool(namespace, key string) (bool, error) {
	s, err := e.String(namespace, key)
	if err != nil {
		return false, err
	}

	return strconv.ParseBool(s)
}

func (e *Configure) YamlDecode(namespace, key string, value any) error {
	return e.Decode(namespace, key, value, yaml.Unmarshal)
}

func (e *Configure) JsonDecode(namespace, key string, value any) error {
	return e.Decode(namespace, key, value, json.Unmarshal)
}

func (e *Configure) Decode(namespace, key string, value any, unmarshalFunc func([]byte, any) error) error {
	d, ok := e.ds[namespace]
	if !ok {
		return ErrNotFound
	}

	b, err := d.Get(namespace, key)
	if err != nil {
		return err
	}

	return unmarshalFunc(b, value)
}

func (e *Configure) Close() {
	for _, v := range e.ds {
		v.Close()
	}
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	n := len(s)
	if n < 2 {
		return s
	}

	if s[0] == '"' && s[n-1] == '"' {
		if unquoted, err := strconv.Unquote(s); err == nil {
			return unquoted
		}
		return s[1 : n-1]
	}

	if s[0] == '\'' && s[n-1] == '\'' {
		return s[1 : n-1]
	}

	return s
}
