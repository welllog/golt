package config

import (
	"testing"
)

func TestConfigure_CloseSequentially(t *testing.T) {
	// One driver often serves several namespaces (test/demo1 | test/demo2),
	// so Close visits it repeatedly; repeated sequential Close must be a no-op.
	// Concurrent Close is out of contract.
	engine := initConfigure(t)
	engine.Close()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("second Close panicked: %v", r)
		}
	}()
	engine.Close()
}
