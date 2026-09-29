package srvhttp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/welllog/golib/testz"
)

func TestRouter_StaticPrefix(t *testing.T) {
	dir := t.TempDir()
	testz.Nil(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0644))

	engine := New()
	engine.Static("/static", dir, false)

	srv := httptest.NewServer(engine)
	defer srv.Close()

	rsp, err := srv.Client().Get(srv.URL + "/static/hello.txt")
	testz.Nil(t, err)
	defer rsp.Body.Close()
	testz.Equal(t, http.StatusOK, rsp.StatusCode)

	// sibling path must not match the /static prefix
	rsp2, err := srv.Client().Get(srv.URL + "/staticfoo")
	testz.Nil(t, err)
	defer rsp2.Body.Close()
	testz.Equal(t, http.StatusNotFound, rsp2.StatusCode)
}

func TestRouter_StaticFileExactMatch(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "f.txt")
	testz.Nil(t, os.WriteFile(fp, []byte("hi"), 0644))

	engine := New()
	engine.StaticFile("/sf", fp)

	srv := httptest.NewServer(engine)
	defer srv.Close()

	rsp, err := srv.Client().Get(srv.URL + "/sf")
	testz.Nil(t, err)
	defer rsp.Body.Close()
	testz.Equal(t, http.StatusOK, rsp.StatusCode)

	// used to serve the file for sibling paths too, via PathPrefix
	rsp2, err := srv.Client().Get(srv.URL + "/sfX")
	testz.Nil(t, err)
	defer rsp2.Body.Close()
	testz.Equal(t, http.StatusNotFound, rsp2.StatusCode)
}

func TestRouter_UseStd(t *testing.T) {
	engine := New()

	var order []string
	engine.UseStd(
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "std1")
				next.ServeHTTP(w, r)
			})
		},
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "std2")
				// the engine Context must be reachable from std middleware
				if _, ok := w.(*Context); !ok {
					t.Error("writer should be *Context inside std middleware")
				}
				next.ServeHTTP(w, r)
			})
		},
	)
	engine.Use(func(c *Context, next Handler) (any, error) {
		order = append(order, "biz")
		return next(c)
	})
	engine.GET("/ping", func(c *Context) (any, error) {
		order = append(order, "handler")
		return "pong", nil
	})

	srv := httptest.NewServer(engine)
	defer srv.Close()

	rsp, err := srv.Client().Get(srv.URL + "/ping")
	testz.Nil(t, err)
	defer rsp.Body.Close()
	testz.Equal(t, http.StatusOK, rsp.StatusCode)

	want := []string{"std1", "std2", "biz", "handler"}
	if len(order) != len(want) {
		t.Fatalf("execution order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("execution order = %v, want %v", order, want)
		}
	}
}
