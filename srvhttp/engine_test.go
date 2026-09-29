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

func TestEngine_PanicRecovery(t *testing.T) {
	engine := New()
	engine.GET("/boom", func(c *Context) (any, error) {
		panic("kaboom")
	})
	engine.GET("/panic-after-write", func(c *Context) (any, error) {
		c.WriteHeader(http.StatusOK)
		_, _ = c.Write([]byte(`{"data":"partial"}`))
		panic("late panic")
	})

	srv := httptest.NewServer(engine)
	defer srv.Close()

	// plain panic: 500 + unified JSON error, not a connection reset
	rsp, err := srv.Client().Get(srv.URL + "/boom")
	testz.Nil(t, err)
	defer rsp.Body.Close()
	testz.Equal(t, http.StatusInternalServerError, rsp.StatusCode)

	b, err := io.ReadAll(rsp.Body)
	testz.Nil(t, err)
	var m map[string]any
	testz.Nil(t, json.Unmarshal(b, &m))
	testz.Equal(t, float64(unierr.Internal), m["code"])
	testz.Equal(t, "internal server error", m["msg"])

	// panic after the response was written: keep the partial body, no crash
	rsp2, err := srv.Client().Get(srv.URL + "/panic-after-write")
	testz.Nil(t, err)
	defer rsp2.Body.Close()
	testz.Equal(t, http.StatusOK, rsp2.StatusCode)

	b2, err := io.ReadAll(rsp2.Body)
	testz.Nil(t, err)
	testz.Equal(t, `{"data":"partial"}`, string(b2))
}

func TestEngine_PanicRecoveryDebug(t *testing.T) {
	engine := New(WithDebug(true))
	engine.GET("/boom", func(c *Context) (any, error) {
		panic("secret detail")
	})

	srv := httptest.NewServer(engine)
	defer srv.Close()

	rsp, err := srv.Client().Get(srv.URL + "/boom")
	testz.Nil(t, err)
	defer rsp.Body.Close()
	testz.Equal(t, http.StatusInternalServerError, rsp.StatusCode)

	b, err := io.ReadAll(rsp.Body)
	testz.Nil(t, err)
	var m map[string]any
	testz.Nil(t, json.Unmarshal(b, &m))
	testz.Equal(t, "secret detail", m["msg"])
}
