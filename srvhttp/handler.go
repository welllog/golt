package srvhttp

import (
	"encoding/json"
	"errors"
	"net/http"

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

		c.Header().Set("Content-Type", "application/json; charset=utf-8")
		c.WriteHeader(ue.HttpCode())
		if b, mErr := ue.MarshalJSON(); mErr == nil {
			_, _ = c.Write(b)
		} else {
			writeEncodeFallback(c, mErr)
		}
		return
	}

	if response == nil {
		c.WriteHeader(http.StatusNoContent)
		return
	}

	buf := c.Buffer()
	buf.Reset()
	buf.Grow(128)

	buf.WriteString(`{"data":`)
	if encErr := json.NewEncoder(buf).Encode(response); encErr != nil {
		writeEncodeFallback(c, encErr)
		return
	}
	b := buf.Bytes()
	if b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	b = append(b, '}')

	c.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.WriteHeader(200)
	_, _ = c.Write(b)
}

// writeEncodeFallback reports an encoding failure as a 500 JSON error instead
// of emitting broken JSON with a success status.
func writeEncodeFallback(c *Context, err error) {
	ue := unierr.New(unierr.Internal, "response encode failed").SetHttpCode(http.StatusInternalServerError)
	c.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.WriteHeader(ue.HttpCode())
	b, _ := ue.MarshalJSON()
	_, _ = c.Write(b)
}
