package driver

import "errors"

var ErrNotFound = errors.New("not found")

type Driver interface {
	Namespaces() []string
	OnKeyChange(namespace, key string, hook func([]byte) error) bool
	Get(namespace, key string) ([]byte, error)
	GetString(namespace, key string) (string, error)
	Close()
}
