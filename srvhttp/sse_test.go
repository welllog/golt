package srvhttp

import (
	"net/http/httptest"
	"testing"

	"github.com/welllog/golib/testz"
)

func TestEvent_Encode_Types(t *testing.T) {
	cases := []struct {
		name string
		data any
		want string
	}{
		{
			name: "string single line",
			data: "hello",
			want: "data:hello\n\n",
		},
		{
			name: "string multiline",
			data: "line1\nline2",
			want: "data:line1\ndata:line2\n\n",
		},
		{
			name: "raw bytes",
			data: []byte("raw byte content"),
			want: "data:raw byte content\n\n",
		},
		{
			name: "nil data",
			data: nil,
			want: "data:\n\n",
		},
		{
			name: "json struct",
			data: struct {
				Msg string `json:"msg"`
			}{Msg: "ok"},
			want: "data:{\"msg\":\"ok\"}\n\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/sse", nil)
			ctx := NewContext(rec, req)

			err := Event{Data: c.data}.Encode(ctx)
			testz.Nil(t, err)
			testz.Equal(t, c.want, rec.Body.String())
			testz.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
		})
	}
}

func TestContextPool_ReuseAndIsolation(t *testing.T) {
	engine := New()
	engine.GET("/pool-test", func(c *Context) (any, error) {
		c.Set("counter", 1)
		val := c.GetInt("counter")
		testz.Equal(t, 1, val)
		return "ok", nil
	})

	srv := httptest.NewServer(engine)
	defer srv.Close()

	// Make multiple sequential requests to ensure pool recycling works cleanly
	for i := 0; i < 5; i++ {
		rsp, err := srv.Client().Get(srv.URL + "/pool-test")
		testz.Nil(t, err)
		rsp.Body.Close()
		testz.Equal(t, 200, rsp.StatusCode)
	}
}
