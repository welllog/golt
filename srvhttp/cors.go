package srvhttp

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

var DefaultCorsConfig = CorsConfig{
	AllowAllOrigins: true,
	AllowMethods: []string{
		http.MethodGet, http.MethodPost,
		http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions,
	},
	AllowHeaders: []string{"Origin", "Content-Length", "Content-Type"},
	MaxAge:       12 * time.Hour,
}

type CorsConfig struct {
	AllowPrivateNetwork bool
	AllowCredentials    bool
	AllowAllOrigins     bool
	AllowOrigins        []string
	AllowMethods        []string
	AllowHeaders        []string
	ExposeHeaders       []string
	MaxAge              time.Duration
	normalHeaders       http.Header
	preflightHeaders    http.Header
}

func (c *CorsConfig) apply(request *http.Request, writer http.ResponseWriter) {
	origin := request.Header.Get("Origin")

	if notCors(origin, request.Host) {
		writer.Header().Set("Vary", "Origin")
		return
	}

	if !c.validateOrigin(origin) {
		writer.Header().Set("Vary", "Origin")
		return
	}

	h := writer.Header()
	setHeaders := c.normalHeaders
	if request.Method == http.MethodOptions {
		setHeaders = c.preflightHeaders
	}

	for k, v := range setHeaders {
		h[k] = slices.Clone(v)
	}

	if !c.AllowAllOrigins || c.AllowCredentials {
		h.Set("Access-Control-Allow-Origin", origin)
	}
}

func (c *CorsConfig) validateOrigin(origin string) bool {
	if c.AllowAllOrigins {
		return true
	}

	for _, o := range c.AllowOrigins {
		if o == origin || (o[0] == '*' && matchWildcardOrigin(origin, o[1:])) {
			return true
		}
	}

	return false
}

// matchWildcardOrigin reports whether origin ends with suffix at a host
// boundary, so "*127.0.0.1" matches "https://127.0.0.1" but not
// "https://evil127.0.0.1".
func matchWildcardOrigin(origin, suffix string) bool {
	if !strings.HasSuffix(origin, suffix) || len(origin) == len(suffix) {
		return false
	}

	if suffix[0] == '.' {
		return true // the dot boundary is part of the suffix
	}

	switch origin[len(origin)-len(suffix)-1] {
	case '.', '/':
		return true
	}
	return false
}

func (c *CorsConfig) init() {
	for _, o := range c.AllowOrigins {
		if o == "*" {
			c.AllowAllOrigins = true
			c.AllowOrigins = nil
			break
		}

		if o == "" {
			panic("AllowOrigin can not be empty string")
		}

		count := strings.Count(o, "*")
		if count > 1 {
			panic("AllowOrigin can not contain more than one *")
		}

		if count == 1 && o[0] != '*' {
			panic("AllowOrigin only support * at the beginning")
		}
	}

	c.genNormalHeaders()
	c.genPreflightHeaders()
}

func (c *CorsConfig) genNormalHeaders() {
	h := http.Header{}

	if c.AllowCredentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}

	if len(c.ExposeHeaders) > 0 {
		h.Set(
			"Access-Control-Expose-Headers",
			strings.Join(normalize(c.ExposeHeaders, http.CanonicalHeaderKey), ","),
		)
	}

	if c.AllowAllOrigins && !c.AllowCredentials {
		h.Set("Access-Control-Allow-Origin", "*")
	} else {
		h.Set("Vary", "Origin")
	}

	c.normalHeaders = h
}

func (c *CorsConfig) genPreflightHeaders() {
	h := http.Header{}

	if c.AllowPrivateNetwork {
		h.Set("Access-Control-Allow-Private-Network", "true")
	}

	if c.AllowCredentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}

	if c.AllowAllOrigins && !c.AllowCredentials {
		h.Set("Access-Control-Allow-Origin", "*")
	} else {
		h.Set("Vary", "Origin")
	}

	if len(c.AllowMethods) > 0 {
		h.Set(
			"Access-Control-Allow-Methods",
			strings.Join(normalize(c.AllowMethods, strings.ToUpper), ","),
		)
	}

	if len(c.AllowHeaders) > 0 {
		h.Set(
			"Access-Control-Allow-Headers",
			strings.Join(normalize(c.AllowHeaders, http.CanonicalHeaderKey), ","),
		)
	}

	if len(c.ExposeHeaders) > 0 {
		h.Set(
			"Access-Control-Expose-Headers",
			strings.Join(normalize(c.ExposeHeaders, http.CanonicalHeaderKey), ","),
		)
	}

	if c.MaxAge > 0 {
		h.Set("Access-Control-Max-Age", strconv.FormatInt(int64(c.MaxAge/time.Second), 10))
	}

	c.preflightHeaders = h
}

func normalize(values []string, fn func(string) string) []string {
	set := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if fn == nil {
			fn = strings.ToLower
		}
		value = fn(value)
		if _, ok := set[value]; !ok {
			set[value] = struct{}{}
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func notCors(origin, host string) bool {
	if origin == "" {
		return true
	}

	// the host part must be non-empty, so a bare "http://" matches nothing
	if len(origin) > len("http://") && origin[:len("http://")] == "http://" {
		return origin[len("http://"):] == host
	}
	if len(origin) > len("https://") && origin[:len("https://")] == "https://" {
		return origin[len("https://"):] == host
	}

	return false
}
