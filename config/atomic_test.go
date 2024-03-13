package config

import (
	"encoding/json"
	"fmt"
)

type workConfig struct {
	work *work
}

func ExampleAtomicStore() {
	engine, err := FromFile("./config.yaml", nil)
	if err != nil {
		panic(err)
	}
	defer engine.Close()

	var c workConfig
	err = AtomicStore(engine, "test/demo1", "work", &c.work, json.Unmarshal)
	if err != nil {
		panic(err)
	}

	fmt.Println(*c.work)
	// Output:
	// {engineer 10000}
}

func ExampleAtomicLoad() {
	engine, err := FromFile("./config.yaml", nil)
	if err != nil {
		panic(err)
	}
	defer engine.Close()

	var c workConfig
	err = AtomicStore(engine, "test/demo1", "work", &c.work, json.Unmarshal)
	if err != nil {
		panic(err)
	}

	w := (*work)(AtomicLoad(&c.work))
	fmt.Println(*w)
	// Output:
	// {engineer 10000}
}
