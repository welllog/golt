package driver

import (
	"encoding/json"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type Decoder func([]byte, any) error

var decoderMap = map[string]Decoder{
	"json": json.Unmarshal,
	"yaml": yaml.Unmarshal,
	"yml":  yaml.Unmarshal,
	"toml": toml.Unmarshal,
}

func RegisterDecoder(format string, fn Decoder) {
	decoderMap[format] = fn
}

func GetDecoder(format string) (Decoder, bool) {
	fn, ok := decoderMap[format]
	return fn, ok
}

func MustGetDecoder(format string) Decoder {
	return decoderMap[format]
}

func GetDecoderOrDefault(format string) Decoder {
	fn, ok := decoderMap[format]
	if ok {
		return fn
	}
	return yaml.Unmarshal
}
