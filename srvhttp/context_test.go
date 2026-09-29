package srvhttp

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/welllog/golib/testz"
)

func TestContext_HijackSuppressFallbackResponse(t *testing.T) {
	engine := New()
	engine.GET("/hijack", func(c *Context) (any, error) {
		conn, _, err := c.Hijack()
		if err != nil {
			return nil, err
		}
		defer conn.Close()

		// write a private-protocol greeting on the raw connection
		_, err = conn.Write([]byte("RAW"))
		return nil, err
	})
	engine.GET("/plain", func(c *Context) (any, error) {
		return "ok", nil
	})

	srv := httptest.NewServer(engine)
	defer srv.Close()

	// hijack route: read the raw bytes and verify no HTTP header was prepended
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	testz.Nil(t, err)
	defer conn.Close()

	req, err := http.NewRequest(http.MethodGet, "http://example.com/hijack", nil)
	testz.Nil(t, err)
	testz.Nil(t, req.Write(conn))

	br := bufio.NewReader(conn)
	head := make([]byte, len("RAW"))
	_, err = io.ReadFull(br, head)
	testz.Nil(t, err)
	testz.Equal(t, "RAW", string(head), "no HTTP response should be written on the hijacked connection")

	// non-hijack route still works normally
	rsp, err := srv.Client().Get(srv.URL + "/plain")
	testz.Nil(t, err)
	defer rsp.Body.Close()
	testz.Equal(t, http.StatusOK, rsp.StatusCode)
}

func TestContext_RouteAccessorsWithoutRoute(t *testing.T) {
	// no route matched (e.g. inside a NotFound handler): used to panic
	// on a nil mux.CurrentRoute result
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	c := NewContext(httptest.NewRecorder(), req)

	testz.Equal(t, "", c.RouteName())
	testz.Equal(t, "", c.PathTemplate())
	testz.Equal(t, "", c.PathRegex())
}
