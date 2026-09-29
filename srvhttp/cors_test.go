package srvhttp

import (
	"net/http/httptest"
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
