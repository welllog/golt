package file

import (
	"gopkg.in/yaml.v3"
)

type field struct {
	content []byte
}

func (f *field) UnmarshalJSON(b []byte) error {
	f.content = append(f.content[:0], b...)
	return nil
}

func (f *field) UnmarshalYAML(value *yaml.Node) error {
	b, err := yaml.Marshal(value)
	if err != nil {
		return err
	}

	f.content = b
	return nil
}
