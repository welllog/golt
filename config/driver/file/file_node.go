package file

import (
	"bytes"
)

type entry struct {
	// content is the raw content of the entry.
	content []byte
	// hook is the hook function that will be executed when the entry is updated.
	hook func([]byte) error
	// hookFlag is the flag that indicates whether the hook function needs to be executed.
	hookFlag bool
}

type fileNode struct {
	dynamic bool
	entries map[string]*entry
}

func (n *fileNode) SetFields(fields map[string]*field) {
	for k, v := range n.entries {
		if _, ok := fields[k]; !ok {
			delete(n.entries, k)
			continue
		}

		if !bytes.Equal(v.content, fields[k].content) {
			v.content = fields[k].content
			if v.hook != nil {
				v.hookFlag = true
			}
		}

		delete(fields, k)
	}

	if n.entries == nil {
		n.entries = make(map[string]*entry, len(fields))
	}

	for k, v := range fields {
		n.entries[k] = &entry{
			content: v.content,
		}

		delete(fields, k)
	}
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
