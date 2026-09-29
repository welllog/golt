package srvhttp

import (
	"context"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/welllog/golib/testz"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	testz.Nil(t, err)
	addr := l.Addr().String()
	testz.Nil(t, l.Close())
	return addr
}

func TestEngine_Run(t *testing.T) {
	engine := New()
	engine.GET("/ping", func(c *Context) (any, error) {
		return "pong", nil
	})

	addr := freeAddr(t)
	go func() {
		if err := engine.Run(addr); err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	}()

	// wait for the listener
	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not start")
		}
	}

	// normal shutdown is not an error: reuse the hijack-free path
	rsp, err := http.Get("http://" + addr + "/ping")
	testz.Nil(t, err)
	_ = rsp.Body.Close()
}

func TestEngine_RunAddrInUse(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	testz.Nil(t, err)
	defer l.Close()

	engine := New()
	err = engine.Run(l.Addr().String())
	if err == nil {
		t.Fatal("expected error for busy address")
	}
}

func TestEngine_RunGracefully(t *testing.T) {
	engine := New()
	engine.GET("/ping", func(c *Context) (any, error) {
		return "pong", nil
	})

	addr := freeAddr(t)
	done := make(chan error, 1)
	go func() {
		done <- engine.RunGracefully(context.Background(), addr, 5*time.Second)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not start")
		}
	}

	// deliver a real SIGTERM to ourselves: RunGracefully should drain and return nil
	testz.Nil(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))

	select {
	case err := <-done:
		testz.Nil(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("RunGracefully did not return after SIGTERM")
	}
}
