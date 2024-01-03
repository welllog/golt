package srvhttp

import (
	"fmt"
	"io"
	"net/http/httptest"

	"github.com/welllog/golt/unierr"
)

func ExampleEngine_ServeHTTP() {
	engine := New()
	engine.POST("/test1", func(c *Context) (any, error) {
		return map[string]string{"greeting": "hello"}, nil
	})

	srv := httptest.NewServer(engine)
	rsp, err := srv.Client().Post(srv.URL+"/test1", "", nil)
	if err != nil {
		fmt.Println("err occurred:", err)
		return
	}
	defer rsp.Body.Close()

	b, _ := io.ReadAll(rsp.Body)
	fmt.Println(string(b))
	// Output:
	// {"data":{"greeting":"hello"}}
}

func ExampleEngine_ServeHTTP_2() {
	engine := New()
	engine.POST("/test2", func(c *Context) (any, error) {
		return nil, unierr.New(1000, "test error")
	})

	srv := httptest.NewServer(engine)
	rsp, err := srv.Client().Post(srv.URL+"/test2", "", nil)
	if err != nil {
		fmt.Println("err occurred:", err)
		return
	}
	defer rsp.Body.Close()

	b, _ := io.ReadAll(rsp.Body)
	fmt.Println(string(b))
	// Output:
	// {"code":1000,"msg":"test error"}
}

func ExampleEngine_ServeHTTP_3() {
	engine := New()
	engine.POST("/test3", func(c *Context) (any, error) {
		return nil, unierr.New(1000, "test error").WithData(map[string]int{"reason": 20})
	})

	srv := httptest.NewServer(engine)
	rsp, err := srv.Client().Post(srv.URL+"/test3", "", nil)
	if err != nil {
		fmt.Println("err occurred:", err)
		return
	}
	defer rsp.Body.Close()

	b, _ := io.ReadAll(rsp.Body)
	fmt.Println(string(b))
	// Output:
	// {"code":1000,"msg":"test error","data":{"reason":20}}
}
