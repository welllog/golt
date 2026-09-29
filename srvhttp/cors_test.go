package srvhttp

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/welllog/golib/testz"
)

func TestNotCors(t *testing.T) {
	cases := []struct {
		origin string
		host   string
		want   bool
	}{
		{"", "any", true},
		{"http://a", "a", true},   // 8 chars: used to be misjudged as cross-origin
		{"http://ab", "ab", true}, // 9 chars: used to be misjudged as cross-origin
		{"https://a", "a", true},
		{"https://example.com", "example.com", true},
		{"http://example.com", "other.com", false},
		{"https://evil.com", "example.com", false},
	}

	for _, c := range cases {
		testz.Equal(t, c.want, notCors(c.origin, c.host), "origin=%q host=%q", c.origin, c.host)
	}
}

func TestValidateOrigin(t *testing.T) {
	cfg := &CorsConfig{AllowOrigins: []string{"*127.0.0.1:5500", "*.example.com", "https://exact.com"}}

	testz.Equal(t, true, cfg.validateOrigin("http://127.0.0.1:5500"))
	testz.Equal(t, true, cfg.validateOrigin("https://127.0.0.1:5500"))
	// used to pass: suffix match without a host boundary
	testz.Equal(t, false, cfg.validateOrigin("https://evil127.0.0.1:5500"))

	testz.Equal(t, true, cfg.validateOrigin("https://a.example.com"))
	testz.Equal(t, false, cfg.validateOrigin("https://aexample.com"))

	testz.Equal(t, true, cfg.validateOrigin("https://exact.com"))
	testz.Equal(t, false, cfg.validateOrigin("https://exact.com.evil.io"))
}

func TestCors_AllowCredentials_With_AllowAllOrigins(t *testing.T) {
	cfg := CorsConfig{
		AllowAllOrigins:  true,
		AllowCredentials: true,
	}
	cfg.init()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/api", nil)
	req.Header.Set("Origin", "http://foo.com")

	cfg.apply(req, rec)

	// Under W3C CORS spec, when AllowCredentials is true, Allow-Origin cannot be '*'
	testz.Equal(t, "http://foo.com", rec.Header().Get("Access-Control-Allow-Origin"))
	testz.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))
	testz.Equal(t, "Origin", rec.Header().Get("Vary"))
}

func TestCors_HeaderIsolation(t *testing.T) {
	cfg := CorsConfig{
		AllowAllOrigins: true,
		AllowHeaders:    []string{"X-Custom-Header"},
	}
	cfg.init()

	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("OPTIONS", "http://example.com/api", nil)
	req1.Header.Set("Origin", "http://foo.com")
	cfg.apply(req1, rec1)

	// Mutate rec1's header slice
	rec1.Header().Add("Access-Control-Allow-Headers", "X-Injected-Header")

	// Apply on a new request; preflightHeaders must not be affected
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("OPTIONS", "http://example.com/api", nil)
	req2.Header.Set("Origin", "http://bar.com")
	cfg.apply(req2, rec2)

	testz.Equal(t, "X-Custom-Header", rec2.Header().Get("Access-Control-Allow-Headers"))
}

func TestCors_VaryNotOverwritten(t *testing.T) {
	cfg := CorsConfig{AllowOrigins: []string{"https://example.com"}}
	cfg.init()

	rec := httptest.NewRecorder()
	// e.g. set by a compression middleware registered before CORS
	rec.Header().Set("Vary", "Accept-Encoding")
	req := httptest.NewRequest("GET", "http://api.com/x", nil)
	req.Header.Set("Origin", "https://example.com")

	cfg.apply(req, rec)
	testz.Equal(t, "Accept-Encoding, Origin", rec.Header().Get("Vary"))

	// applying twice must not duplicate the value
	cfg.apply(req, rec)
	testz.Equal(t, "Accept-Encoding, Origin", rec.Header().Get("Vary"))
}

func TestEngine_CorsOptions(t *testing.T) {
	engine := New()
	engine.UseCors(CorsConfig{
		AllowOrigins: []string{"https://example.com"},
		AllowMethods: []string{http.MethodGet, http.MethodPost},
	})

	var handlerCalls atomic.Int32
	engine.Any("/opts", func(c *Context) (any, error) {
		handlerCalls.Add(1)
		return "ok", nil
	})
	engine.GET("/only-get", func(c *Context) (any, error) {
		return "ok", nil
	})

	srv := httptest.NewServer(engine)
	defer srv.Close()

	// plain OPTIONS without CORS headers reaches the handler
	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/opts", nil)
	rsp, err := srv.Client().Do(req)
	testz.Nil(t, err)
	defer rsp.Body.Close()
	testz.Equal(t, http.StatusOK, rsp.StatusCode)
	testz.Equal(t, int32(1), handlerCalls.Load())

	// cross-origin OPTIONS without Access-Control-Request-Method is not a
	// preflight and must also reach the handler
	req2, _ := http.NewRequest(http.MethodOptions, srv.URL+"/opts", nil)
	req2.Header.Set("Origin", "https://example.com")
	rsp2, err := srv.Client().Do(req2)
	testz.Nil(t, err)
	defer rsp2.Body.Close()
	testz.Equal(t, http.StatusOK, rsp2.StatusCode)
	testz.Equal(t, int32(2), handlerCalls.Load())

	// a real preflight is short-circuited with 204
	req3, _ := http.NewRequest(http.MethodOptions, srv.URL+"/opts", nil)
	req3.Header.Set("Origin", "https://example.com")
	req3.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rsp3, err := srv.Client().Do(req3)
	testz.Nil(t, err)
	defer rsp3.Body.Close()
	testz.Equal(t, http.StatusNoContent, rsp3.StatusCode)
	testz.Equal(t, int32(2), handlerCalls.Load())

	// preflight on a path without an OPTIONS route: the 405 handler
	// answers 204 with CORS headers instead of 405
	req4, _ := http.NewRequest(http.MethodOptions, srv.URL+"/only-get", nil)
	req4.Header.Set("Origin", "https://example.com")
	req4.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rsp4, err := srv.Client().Do(req4)
	testz.Nil(t, err)
	defer rsp4.Body.Close()
	testz.Equal(t, http.StatusNoContent, rsp4.StatusCode)
	testz.Equal(t, "https://example.com", rsp4.Header.Get("Access-Control-Allow-Origin"))

	// plain OPTIONS on the same path is a 405
	req5, _ := http.NewRequest(http.MethodOptions, srv.URL+"/only-get", nil)
	rsp5, err := srv.Client().Do(req5)
	testz.Nil(t, err)
	defer rsp5.Body.Close()
	testz.Equal(t, http.StatusMethodNotAllowed, rsp5.StatusCode)
}
