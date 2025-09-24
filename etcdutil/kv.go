package etcdutil

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/welllog/golib/strz"
	"github.com/welllog/golt/contract"
	"github.com/welllog/olog"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var ErrNotFound = errors.New("not found")

type entry struct {
	// value is the value of the key.
	value string
	// exists is to distinguish the key content is empty or not exists.
	exists bool
}

type Kv struct {
	prefix  string
	entries map[string]*entry
	hooks   map[string][]func([]byte) error

	mu     sync.RWMutex
	client *clientv3.Client
	logger contract.Logger
}

// NewKv creates a new Kv.
func NewKv(prefix string, client *clientv3.Client) *Kv {
	kv := Kv{
		prefix:  prefix,
		entries: make(map[string]*entry, 5),
		hooks:   make(map[string][]func([]byte) error),
		client:  client,
		logger:  olog.DynamicLogger{},
	}
	return &kv
}

func (k *Kv) SetLogger(logger contract.Logger) *Kv {
	k.logger = logger
	return k
}

// Prefix returns the prefix of the keys.
func (k *Kv) Prefix() string {
	return k.prefix
}

// Preload loads all keys with the prefix into the cache.
func (k *Kv) Preload(ctx context.Context) error {
	rsp, err := k.client.Get(ctx, k.prefix, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	l := len(k.prefix)

	k.mu.Lock()
	for _, v := range rsp.Kvs {
		e, ok := k.entries[strz.UnsafeString(v.Key[l:])]
		if !ok {
			// if not exists, create a new entry
			k.entries[string(v.Key[l:])] = &entry{
				value: string(v.Value), exists: true,
			}
			continue
		}

		// entry exists, update content if needed
		if !e.exists || !bytes.Equal(strz.UnsafeBytes(e.value), v.Value) {
			e.value = string(v.Value)
			e.exists = true
		}
	}
	k.mu.Unlock()

	return nil
}

// OnKeyChange registers a hook function to be called when the key changes.
// the key removed from etcd will not trigger the hook.
func (k *Kv) OnKeyChange(key string, hook func([]byte) error) bool {
	key = k.cacheKey(key)

	k.mu.Lock()
	k.hooks[key] = append(k.hooks[key], hook)
	k.mu.Unlock()

	return true
}

// GetString gets the value of the key.
func (k *Kv) GetString(ctx context.Context, key string) (string, error) {
	cacheKey := k.cacheKey(key)
	value, cached, exists := k.getStringFromCache(cacheKey)
	if cached {
		if exists {
			return value, nil
		}

		return "", ErrNotFound
	}

	realKey := k.etcdKey(key)
	b, err := k.GetNoCache(ctx, realKey)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// cached an entry with contentExists=false, to avoid request etcd
			k.cacheNilWhenNotFound(cacheKey)
		}
		return "", err
	}

	value = string(b)

	k.mu.Lock()
	_, ok := k.entries[cacheKey]
	if !ok {
		k.entries[cacheKey] = &entry{value: value, exists: true}
	}
	k.mu.Unlock()

	return value, nil
}

// Get gets the value of the key.
func (k *Kv) Get(ctx context.Context, key string) ([]byte, error) {
	cacheKey := k.cacheKey(key)
	value, cached, exists := k.getStringFromCache(cacheKey)
	if cached {
		if exists {
			return []byte(value), nil
		}

		return nil, ErrNotFound
	}

	realKey := k.etcdKey(key)
	b, err := k.GetNoCache(ctx, realKey)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// cached an entry with contentExists=false, to avoid request etcd
			k.cacheNilWhenNotFound(cacheKey)
		}
		return nil, err
	}

	k.cacheWhenNotFound(cacheKey, b)
	return b, nil
}

// UnsafeGet gets the value of the key.
// the []byte of return maybe not safe, if you want to use it for a long time, please copy it.
func (k *Kv) UnsafeGet(ctx context.Context, key string) ([]byte, error) {
	cacheKey := k.cacheKey(key)
	value, cached, exists := k.getStringFromCache(cacheKey)
	if cached {
		if exists {
			return strz.UnsafeBytes(value), nil
		}

		return nil, ErrNotFound
	}

	realKey := k.etcdKey(key)
	b, err := k.GetNoCache(ctx, realKey)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// cached an entry with contentExists=false, to avoid request etcd
			k.cacheNilWhenNotFound(cacheKey)
		}
		return nil, err
	}

	k.cacheWhenNotFound(cacheKey, b)
	return b, nil
}

// GetNoCache gets the value of the key without using the cache.
func (k *Kv) GetNoCache(ctx context.Context, key string) ([]byte, error) {
	key = k.etcdKey(key)

	rsp, err := k.client.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if len(rsp.Kvs) == 0 {
		return nil, ErrNotFound
	}

	return rsp.Kvs[0].Value, nil
}

// Len returns the number of entries in the cache.
func (k *Kv) Len() int {
	k.mu.RLock()
	n := len(k.entries)
	k.mu.RUnlock()

	return n
}

// Handle handles the etcd event.
func (k *Kv) Handle(event *clientv3.Event) {
	switch event.Type {
	case clientv3.EventTypePut:
		var diff bool

		key := strz.UnsafeString(event.Kv.Key[len(k.prefix):])
		k.mu.Lock()
		e, ok := k.entries[key]
		if !ok {
			k.mu.Unlock()
			return
		}

		if !e.exists || !bytes.Equal(strz.UnsafeBytes(e.value), event.Kv.Value) {
			diff = true
			e.value = string(event.Kv.Value)
			e.exists = true
		}
		k.mu.Unlock()

		if diff {
			k.logger.Debugf("key %s changed", key)

			k.mu.RLock()
			for _, hook := range k.hooks[key] {
				if err := hook(event.Kv.Value); err != nil {
					k.logger.Warnf("key %s hook failed: %s", key, err.Error())
				}
			}
			k.mu.RUnlock()
		}
	case clientv3.EventTypeDelete:
		k.mu.Lock()
		e, ok := k.entries[strz.UnsafeString(event.Kv.Key[len(k.prefix):])]
		if ok {
			e.value = ""
			e.exists = false
		}
		k.mu.Unlock()
	default:
	}
}

// getStringFromCache gets the value of the key from the cache.
func (k *Kv) getStringFromCache(key string) (value string, cached, exists bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	e, ok := k.entries[key]
	if !ok {
		return "", false, false
	}

	return e.value, true, e.exists
}

// cacheNilWhenNotFound caches an entry with exists=false when key not found in cache, to avoid request etcd.
// if return true, means the key not exists in the cache and cached successfully.
// if return false, means the key exists in the cache and cached failed.
func (k *Kv) cacheNilWhenNotFound(key string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()

	_, ok := k.entries[key]
	if ok {
		return false
	}

	k.entries[key] = &entry{}
	return true
}

// cacheWhenNotFound caches an entry when key not found in cache.
func (k *Kv) cacheWhenNotFound(key string, value []byte) bool {
	k.mu.Lock()
	defer k.mu.Unlock()

	_, ok := k.entries[key]
	if ok {
		return false
	}

	k.entries[key] = &entry{value: string(value), exists: true}
	return true
}

// cacheKey returns the key without the prefix.
func (k *Kv) cacheKey(key string) string {
	if strings.HasPrefix(key, k.prefix) {
		return key[len(k.prefix):]
	}
	return key
}

// etcdKey returns the key with the prefix.
func (k *Kv) etcdKey(key string) string {
	if strings.HasPrefix(key, k.prefix) {
		return key
	}
	return k.prefix + key
}
