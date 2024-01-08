package file

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

type field struct {
	value []byte
}

func (f *field) UnmarshalJSON(b []byte) error {
	f.value = append(f.value[:0], b...)
	return nil
}

func (f *field) UnmarshalYAML(value *yaml.Node) error {
	b, err := yaml.Marshal(value)
	if err != nil {
		return err
	}

	f.value = bytes.TrimRight(b, "\n")
	return nil
}
