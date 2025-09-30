package file

import (
	"bytes"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type field struct {
	value []byte
}

func (f *field) UnmarshalText(text []byte) error {
	f.value = append(f.value[:0], text...)
	return nil
}

func (f *field) UnmarshalBinary(data []byte) error {
	f.value = append(f.value[:0], data...)
	return nil
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

func (f *field) UnmarshalTOML(data any) error {
	switch val := data.(type) {
	case string:
		f.value = append(f.value[:0], strings.TrimRight(val, "\n")...)
	case []byte:
		f.value = append(f.value[:0], bytes.TrimRight(val, "\n")...)
	default:
		b, err := toml.Marshal(data)
		if err != nil {
			return err
		}
		f.value = append(f.value[:0], bytes.TrimRight(b, "\n")...)
	}
	return nil
}
