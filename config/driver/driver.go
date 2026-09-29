package driver

import (
	"context"
	"errors"
)

var (
	// ErrNotFound reports an unknown namespace or a missing key.
	ErrNotFound = errors.New("not found")
	// ErrNotWatchable reports a namespace whose watch is not enabled in the
	// meta config.
	ErrNotWatchable = errors.New("not watchable, enable watch in the meta config")
)

type Driver interface {
	Namespaces() []string

	// OnKeyChange registers a hook to run when the key's value changes.
	// It returns an error wrapping ErrNotFound when the namespace is unknown,
	// or ErrNotWatchable when watch is not enabled for the namespace.
	// Whether never-loaded keys receive notifications is driver-specific:
	// the file driver tracks all keys at startup, while the etcd driver only
	// tracks keys already accessed (Get/GetString, including not-found, or
	// Preload).
	OnKeyChange(namespace, key string, hook func([]byte) error) error
	Get(ctx context.Context, namespace, key string) ([]byte, error)
	GetString(ctx context.Context, namespace, key string) (string, error)
	Close()
}
