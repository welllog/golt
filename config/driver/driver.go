package driver

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("not found")

type Driver interface {
	Namespaces() []string
	OnKeyChange(namespace, key string, hook func([]byte) error) bool
	Get(ctx context.Context, namespace, key string) ([]byte, error)
	GetString(ctx context.Context, namespace, key string) (string, error)
	Close()
}
