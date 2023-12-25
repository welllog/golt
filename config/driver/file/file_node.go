package file

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/welllog/golt/config/driver"
	"gopkg.in/yaml.v3"
)

var _buf = bytes.NewBuffer(make([]byte, 0, 1024))

type entry struct {
	// content is the raw content of the entry.
	content []byte
	// hook is the hook function that will be executed when the entry is updated.
	hook func([]byte) error
	// hookFlag is the flag that indicates whether the hook function needs to be executed.
	hookFlag bool
}

func (e *entry) UnmarshalJSON(b []byte) error {
	if bytes.Equal(e.content, b) {
		return nil
	}

	e.content = append(e.content[:0], b...)
	if e.hook != nil {
		e.hookFlag = true
	}
	return nil
}

func (e *entry) UnmarshalYAML(value *yaml.Node) error {
	_buf.Reset()
	err := yaml.NewEncoder(_buf).Encode(value)
	if err != nil {
		return err
	}

	b := _buf.Bytes()
	if bytes.Equal(e.content, b) {
		return nil
	}

	e.content = append(e.content[:0], b...)
	if e.hook != nil {
		e.hookFlag = true
	}
	return nil
}

type fileNode struct {
	dynamic bool
	entries map[string]*entry
}

func (n *fileNode) Load(path string) error {
	var fn func([]byte, any) error

	ext := filepath.Ext(path)
	switch ext {
	case ".json":
		fn = json.Unmarshal
	case ".yaml":
		fn = yaml.Unmarshal
	default:
		return driver.ErrUnsupportedFormat
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if n.entries == nil {
		n.entries = make(map[string]*entry, 5)
	}

	return fn(b, &n.entries)
}

func (n *fileNode) RegisterHook(key string, hook func([]byte) error) bool {
	e, ok := n.entries[key]
	if !ok {
		return false
	}

	e.hook = hook
	return true
}

func (n *fileNode) ExecuteHook() {
	for _, e := range n.entries {
		if e.hookFlag && e.hook != nil {
			if err := e.hook(e.content); err != nil {
				// TODO: log
			}
		}
		e.hookFlag = false
	}
}

func (n *fileNode) Get(key string) ([]byte, bool) {
	e, ok := n.entries[key]
	if !ok {
		return nil, false
	}
	return e.content, true
}
