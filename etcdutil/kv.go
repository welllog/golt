package etcdutil

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

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

	// watched marks whether this Kv is attached to a live Watcher. The
	// negative cache is only sound with one: without watch events to flip a
	// cached miss, it would report not-found forever.
	watched atomic.Bool

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

// SetWatched marks this Kv as attached to a live Watcher. It gates the
// negative cache: a cached miss can only be healed by a watch event.
func (k *Kv) SetWatched() *Kv {
	k.watched.Store(true)
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
// The key removed from etcd will not trigger the hook.
// The hook list is append-only: Handle snapshots it without copying, so
// removing or replacing hooks in place would break that snapshot.
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

	value, _, err := k.getAndCache(ctx, key, cacheKey)
	return value, err
}

// Get gets the value of the key.
// The value is cached on first read. Updates are picked up only when the Kv
// is watched (see SetWatched); otherwise use GetNoCache for fresh values.
func (k *Kv) Get(ctx context.Context, key string) ([]byte, error) {
	cacheKey := k.cacheKey(key)
	value, cached, exists := k.getStringFromCache(cacheKey)
	if cached {
		if exists {
			return []byte(value), nil
		}

		return nil, ErrNotFound
	}

	_, b, err := k.getAndCache(ctx, key, cacheKey)
	return b, err
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

	value, _, err := k.getAndCache(ctx, key, cacheKey)
	if err != nil {
		return nil, err
	}

	return strz.UnsafeBytes(value), nil
}

// getAndCache fetches the key from etcd and caches it. It returns both the
// cached string and the raw bytes so callers convert without an extra copy:
// exactly one string(b) conversion happens, for the cache.
func (k *Kv) getAndCache(ctx context.Context, key, cacheKey string) (string, []byte, error) {
	b, err := k.GetNoCache(ctx, k.etcdKey(key))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// cached an entry with contentExists=false, to avoid request etcd
			k.cacheNilWhenNotFound(cacheKey)
		}
		return "", nil, err
	}

	value := string(b)
	k.mu.Lock()
	if _, ok := k.entries[cacheKey]; !ok {
		k.entries[cacheKey] = &entry{value: value, exists: true}
	}
	k.mu.Unlock()

	return value, b, nil
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
		hooks := k.hooks[key]
		if !ok && len(hooks) == 0 {
			k.mu.Unlock()
			return
		}

		if ok {
			if !e.exists || !bytes.Equal(strz.UnsafeBytes(e.value), event.Kv.Value) {
				diff = true
				e.value = string(event.Kv.Value)
				e.exists = true
			}
		} else {
			if event.PrevKv == nil || !bytes.Equal(event.PrevKv.Value, event.Kv.Value) {
				diff = true
			}
			k.entries[key] = &entry{value: string(event.Kv.Value), exists: true}
		}

		// slice-header snapshot only: OnKeyChange appends under k.mu, so later
		// registrations stay invisible to this snapshot without copying
		if !diff {
			hooks = nil
		}
		k.mu.Unlock()

		if !diff {
			return
		}

		k.logger.Debugf("key %s changed", key)

		// hooks run without k.mu held, so they may call Get/OnKeyChange
		for _, hook := range hooks {
			if err := hook(event.Kv.Value); err != nil {
				k.logger.Warnf("key %s hook failed: %s", key, err.Error())
			}
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
// Skipped when the Kv is not watched: without events there is no way to flip
// the cached miss, so it would stick forever.
// if return true, means the key not exists in the cache and cached successfully.
// if return false, means the key exists in the cache or caching is skipped.
func (k *Kv) cacheNilWhenNotFound(key string) bool {
	if !k.watched.Load() {
		return false
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	_, ok := k.entries[key]
	if ok {
		return false
	}

	k.entries[key] = &entry{}
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
