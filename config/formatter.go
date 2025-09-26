package config

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

var formatterMap = map[string]func(b []byte, dst any) error{
	"json": json.Unmarshal,
	"yaml": yaml.Unmarshal,
	"yml":  yaml.Unmarshal,
}

func RegisterFormatUnmarshaler(format string, fn func(b []byte, dst any) error) {
	formatterMap[format] = fn
}

type formatter struct {
	f string
}

func (f formatter) Decode(b []byte, dst any) error {
	if fn, ok := formatterMap[f.f]; ok {
		return fn(b, dst)
	}

	return yaml.Unmarshal(b, dst)
}
