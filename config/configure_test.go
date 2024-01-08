package config

import (
	"testing"
)

type addr struct {
	Province string
	City     string
	Street   string
}

type work struct {
	Title  string
	Salary int
}

func TestEngine_Get(t *testing.T) {
	engine, err := FromFile("./config.yaml", nil)
	if err != nil {
		t.Fatal(err)
	}

	engine.Close()
}
