package srvhttp

import (
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
