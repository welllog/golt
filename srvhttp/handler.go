package srvhttp

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/welllog/golt/unierr"
)

type Handler func(*Context) (any, error)

type Middleware func(*Context, Handler) (any, error)

type ResponseFunc func(response any, err error, c *Context)

func defResponseFunc(response any, err error, c *Context) {
	if err != nil {
		var ue *unierr.Error
		if !errors.As(err, &ue) {
			ue = unierr.New(unierr.UnKnown, err.Error())
		}

		c.WriteHeader(ue.HttpCode())
		b, _ := ue.MarshalJSON()
		_, _ = c.Write(b)
		return
	}

	var buf bytes.Buffer
	buf.Grow(128)

	buf.WriteString(`{"data":`)
	_ = json.NewEncoder(&buf).Encode(response)
	b := buf.Bytes()
	if b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	b = append(b, '}')

	c.WriteHeader(200)
	_, _ = c.Write(b)
}
