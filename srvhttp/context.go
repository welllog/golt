package srvhttp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

type Context struct {
	context.Context
	Request        *http.Request
	responseWriter http.ResponseWriter
	status         int
	size           int
	values         map[string]any
	mu             sync.RWMutex
	rsp            any
	err            error
	buf            bytes.Buffer
}

// NewContext returns a new Context instance.
func NewContext(rsp http.ResponseWriter, req *http.Request) *Context {
	return &Context{
		Context:        req.Context(),
		Request:        req,
		responseWriter: rsp,
	}
}

// Set sets the value associated with key.
func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	if c.values == nil {
		c.values = make(map[string]any, 2)
	}
	c.values[key] = value
	c.mu.Unlock()
}

// Get returns the value associated with key.
func (c *Context) Get(key string) (any, bool) {
	c.mu.RLock()
	v, ok := c.values[key]
	c.mu.RUnlock()

	return v, ok
}

// Value returns the value associated with key.
func (c *Context) Value(key any) any {
	s, ok := key.(string)
	if ok {
		c.mu.RLock()
		v, ok := c.values[s]
		c.mu.RUnlock()

		if ok {
			return v
		}
	}

	return c.Context.Value(key)
}

// PathParams returns the path parameters associated with the request.
func (c *Context) PathParams() map[string]string {
	return mux.Vars(c.Request)
}

// RouteName returns the name of the route matched for the current request, if any.
func (c *Context) RouteName() string {
	return mux.CurrentRoute(c.Request).GetName()
}

// PathTemplate returns the path template used to match the current request, if any.
func (c *Context) PathTemplate() string {
	tpl, _ := mux.CurrentRoute(c.Request).GetPathTemplate()
	return tpl
}

// PathRegex returns the path regex used to match the current request, if any.
func (c *Context) PathRegex() string {
	rgx, _ := mux.CurrentRoute(c.Request).GetPathRegexp()
	return rgx
}

// ClientIP implements a best effort algorithm to return the real client IP
func (c *Context) ClientIP() string {
	return ClientIP(c.Request)
}

// Header returns the response header
func (c *Context) Header() http.Header {
	return c.responseWriter.Header()
}

// WriteHeader writes the response header
func (c *Context) WriteHeader(s int) {
	c.status = s
	c.responseWriter.WriteHeader(s)
}

// Write writes the response body
func (c *Context) Write(b []byte) (int, error) {
	if !c.Written() {
		// The status will be StatusOK if WriteHeader has not been called yet
		c.WriteHeader(http.StatusOK)
	}
	size, err := c.responseWriter.Write(b)
	c.size += size
	return size, err
}

// Status returns the response status code
func (c *Context) Status() int {
	return c.status
}

// Size returns the response size
func (c *Context) Size() int {
	return c.size
}

// Written returns whether or not the response for this context has been written.
func (c *Context) Written() bool {
	return c.status != 0
}

// Hijack implements the http.Hijacker interface to allow an HTTP handler to take over the connection.
func (c *Context) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := c.responseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("the ResponseWriter doesn't support the Hijacker interface")
	}
	return hijacker.Hijack()
}

// Flush implements the http.Flusher interface to allow an HTTP handler to flush buffered data to the client.
func (c *Context) Flush() {
	flusher, ok := c.responseWriter.(http.Flusher)
	if ok {
		if !c.Written() {
			// The status will be StatusOK if WriteHeader has not been called yet
			c.WriteHeader(http.StatusOK)
		}
		flusher.Flush()
	}
}

// File sends a response with the content of the file.
func (c *Context) File(filepath string) {
	http.ServeFile(c, c.Request, filepath)
}

// SSEvent sends a Server-Sent Event.
func (c *Context) SSEvent(name string, message any) error {
	return Event{
		Event: name,
		Data:  message,
	}.Encode(c)
}

// Buffer returns the Buffer associated with the Context.
func (c *Context) Buffer() *bytes.Buffer {
	return &c.buf
}

// GetString returns the value associated with the key as a string.
func (c *Context) GetString(key string) (s string) {
	if val, ok := c.Get(key); ok && val != nil {
		s, _ = val.(string)
	}
	return
}

// GetBool returns the value associated with the key as a boolean.
func (c *Context) GetBool(key string) (b bool) {
	if val, ok := c.Get(key); ok && val != nil {
		b, _ = val.(bool)
	}
	return
}

// GetInt returns the value associated with the key as an integer.
func (c *Context) GetInt(key string) (i int) {
	if val, ok := c.Get(key); ok && val != nil {
		i, _ = val.(int)
	}
	return
}

// GetInt64 returns the value associated with the key as an integer.
func (c *Context) GetInt64(key string) (i64 int64) {
	if val, ok := c.Get(key); ok && val != nil {
		i64, _ = val.(int64)
	}
	return
}

// GetUint returns the value associated with the key as an unsigned integer.
func (c *Context) GetUint(key string) (ui uint) {
	if val, ok := c.Get(key); ok && val != nil {
		ui, _ = val.(uint)
	}
	return
}

// GetUint64 returns the value associated with the key as an unsigned integer.
func (c *Context) GetUint64(key string) (ui64 uint64) {
	if val, ok := c.Get(key); ok && val != nil {
		ui64, _ = val.(uint64)
	}
	return
}

// GetFloat64 returns the value associated with the key as a float64.
func (c *Context) GetFloat64(key string) (f64 float64) {
	if val, ok := c.Get(key); ok && val != nil {
		f64, _ = val.(float64)
	}
	return
}

// GetTime returns the value associated with the key as time.
func (c *Context) GetTime(key string) (t time.Time) {
	if val, ok := c.Get(key); ok && val != nil {
		t, _ = val.(time.Time)
	}
	return
}

// GetDuration returns the value associated with the key as a duration.
func (c *Context) GetDuration(key string) (d time.Duration) {
	if val, ok := c.Get(key); ok && val != nil {
		d, _ = val.(time.Duration)
	}
	return
}

// GetStringSlice returns the value associated with the key as a slice of strings.
func (c *Context) GetStringSlice(key string) (ss []string) {
	if val, ok := c.Get(key); ok && val != nil {
		ss, _ = val.([]string)
	}
	return
}

// GetStringMap returns the value associated with the key as a map of interfaces.
func (c *Context) GetStringMap(key string) (sm map[string]any) {
	if val, ok := c.Get(key); ok && val != nil {
		sm, _ = val.(map[string]any)
	}
	return
}

// GetStringMapString returns the value associated with the key as a map of strings.
func (c *Context) GetStringMapString(key string) (sms map[string]string) {
	if val, ok := c.Get(key); ok && val != nil {
		sms, _ = val.(map[string]string)
	}
	return
}

// GetStringMapStringSlice returns the value associated with the key as a map to a slice of strings.
func (c *Context) GetStringMapStringSlice(key string) (smss map[string][]string) {
	if val, ok := c.Get(key); ok && val != nil {
		smss, _ = val.(map[string][]string)
	}
	return
}

func ClientIP(r *http.Request) string {
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	ip := strings.TrimSpace(strings.Split(xForwardedFor, ",")[0])
	if ip != "" {
		return ip
	}

	ip = strings.TrimSpace(r.Header.Get("X-Real-Ip"))
	if ip != "" {
		return ip
	}

	if ip, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return ip
	}
	return ""
}
