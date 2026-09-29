package srvhttp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/welllog/golib/testz"
	"github.com/welllog/golt/unierr"
)

func TestDefResponseFunc_EncodeFailure(t *testing.T) {
	engine := New()
	engine.GET("/bad", func(c *Context) (any, error) {
		// channels are not JSON-serializable
		return map[string]any{"ch": make(chan int)}, nil
	})

	srv := httptest.NewServer(engine)
	defer srv.Close()

	rsp, err := srv.Client().Get(srv.URL + "/bad")
	testz.Nil(t, err)
	defer rsp.Body.Close()

	testz.Equal(t, http.StatusInternalServerError, rsp.StatusCode)

	b, err := io.ReadAll(rsp.Body)
	testz.Nil(t, err)

	var m map[string]any
	testz.Nil(t, json.Unmarshal(b, &m), "fallback body must be valid JSON, got: %s", b)
	testz.Equal(t, float64(unierr.Internal), m["code"])
}
