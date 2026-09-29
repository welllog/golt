package file

import (
	"bytes"

	"github.com/welllog/golib/strz"
	"github.com/welllog/golt/config/driver"
)

type entry struct {
	// value is the value of the key.
	value string
	// hooks is a slice of hook functions that will be executed when the value is updated.
	hooks []func([]byte) error

	// exists is used to distinguish whether the value is an empty string or key does not exist.
	exists bool
	// hookFlag is the flag that indicates whether the hook function needs to be executed.
	hookFlag bool
}

type fileNode struct {
	watch   bool
	entries map[string]*entry
}

// CacheFrom caches the fields into the node.
func (n *fileNode) CacheFrom(fields map[string]*field) {
	if n.entries == nil {
		n.entries = make(map[string]*entry, len(fields))
	}

	for k, v := range fields {
		var value []byte
		if v != nil && len(v.value) > 0 {
			value = v.value
		}

		e, ok := n.entries[k]
		if ok {
			if !e.exists || !bytes.Equal(strz.UnsafeBytes(e.value), value) {
				e.exists = true
				e.value = string(value)

				if len(e.hooks) > 0 {
					e.hookFlag = true
				}
			}

			continue
		}

		n.entries[k] = &entry{
			value:  string(value),
			exists: true,
		}
	}

	for k, v := range n.entries {
		_, ok := fields[k]
		if !ok {
			if len(v.hooks) == 0 {
				delete(n.entries, k)
			} else {
				v.value = ""
				v.exists = false
			}
		}
	}
}

// OnKeyChange registers a hook function that will be executed when the value of the key is updated.
// the key removed will not be executed.
func (n *fileNode) OnKeyChange(key string, hook func([]byte) error) error {
	if !n.watch {
		return driver.ErrNotWatchable
	}

	e, ok := n.entries[key]
	if !ok {
		e = &entry{}
		n.entries[key] = e
	}

	e.hooks = append(e.hooks, hook)
	return nil
}

// pendingHook is a collected hook execution: run it after releasing the driver lock.
type pendingHook struct {
	key   string
	value []byte
	hooks []func([]byte) error
}

// CollectHooks collects the hook executions pending for the keys in fields
// and clears their hook flags. It must run with the driver write lock held;
// execute the returned hooks only after releasing the lock. dst is reused to
// avoid allocating per refresh. The hooks slices are snapshots, not copies:
// OnKeyChange only appends, so concurrent registrations stay invisible.
func (n *fileNode) CollectHooks(fields map[string]*field, dst []pendingHook) []pendingHook {
	for k, v := range fields {
		e, ok := n.entries[k]
		if !ok || !e.hookFlag || len(e.hooks) == 0 {
			continue
		}

		var value []byte
		if v != nil {
			value = v.value
		}

		e.hookFlag = false
		dst = append(dst, pendingHook{key: k, value: value, hooks: e.hooks})
	}

	return dst
}

// UnsafeGet returns the value of the key.
func (n *fileNode) UnsafeGet(key string) ([]byte, bool) {
	e, ok := n.entries[key]
	if !ok {
		return nil, false
	}

	if !e.exists {
		return nil, false
	}

	return strz.UnsafeBytes(e.value), true
}

// GetString returns the value of the key.
func (n *fileNode) GetString(key string) (string, bool) {
	e, ok := n.entries[key]
	if !ok {
		return "", false
	}

	if !e.exists {
		return "", false
	}

	return e.value, true
}
