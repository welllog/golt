package srvhttp

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"sync"

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
}

func NewContext(rsp http.ResponseWriter, req *http.Request) *Context {
	return &Context{
		Context:        req.Context(),
		Request:        req,
		responseWriter: rsp,
	}
}

func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	if c.values == nil {
		c.values = make(map[string]any, 2)
	}
	c.values[key] = value
	c.mu.Unlock()
}

func (c *Context) Get(key string) any {
	c.mu.RLock()
	v := c.values[key]
	c.mu.RUnlock()

	return v
}

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

func (c *Context) PathParams() map[string]string {
	return mux.Vars(c.Request)
}

func (c *Context) Header() http.Header {
	return c.responseWriter.Header()
}

func (c *Context) WriteHeader(s int) {
	c.status = s
	c.responseWriter.WriteHeader(s)
}

func (c *Context) Write(b []byte) (int, error) {
	if !c.Written() {
		// The status will be StatusOK if WriteHeader has not been called yet
		c.WriteHeader(http.StatusOK)
	}
	size, err := c.responseWriter.Write(b)
	c.size += size
	return size, err
}

func (c *Context) Status() int {
	return c.status
}

func (c *Context) Size() int {
	return c.size
}

func (c *Context) Written() bool {
	return c.status != 0
}

func (c *Context) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := c.responseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("the ResponseWriter doesn't support the Hijacker interface")
	}
	return hijacker.Hijack()
}

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
