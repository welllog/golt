package driver

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrUnsupportedFormat = errors.New("unsupported config file format")
)

type Driver interface {
	Namespaces() []string
	RegisterHook(namespace, key string, hook func([]byte) error) bool
	Get(namespace, key string) ([]byte, error)
	UnsafeGet(namespace, key string) ([]byte, error)
	Decode(namespace, key string, value any, unmarshalFunc func([]byte, any) error) error
	Close()
}
