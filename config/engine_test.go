package config

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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
	engine, err := EngineFromFile("./config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	b, err := engine.UnsafeGet("test/demo1", "name")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(b))

	b, err = engine.UnsafeGet("test/demo2", "name")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(b))

	var a addr
	err = engine.Decode("test/demo1", "addr", &a, yaml.Unmarshal)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(a)

	var w work
	err = engine.Decode("test/demo2", "work", &w, json.Unmarshal)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(w)

	b, err = engine.Get("test/demo3", "messages")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(b))

	engine.RegisterHook("test/demo2", "addr", func(b []byte) error {
		fmt.Println(string(b))
		return nil
	})
	time.Sleep(time.Second * 30)

	err = engine.Decode("test/demo1", "addr", &a, yaml.Unmarshal)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(a)

	engine.Close()
}
