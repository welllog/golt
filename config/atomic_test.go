package config

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/welllog/golib/testz"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type workConfig struct {
	work *work
}

func ExampleAtomicStore() {
	engine, err := FromFile("./etc/config.yaml", WithCustomEtcdClient(&clientv3.Client{
		KV:      &testKV{},
		Watcher: &testWatcher{},
	}))
	if err != nil {
		panic(err)
	}

	var c workConfig
	err = AtomicStore(context.Background(), engine, "test/demo1", "work", &c.work, json.Unmarshal)
	if err != nil {
		panic(err)
	}

	fmt.Println(*c.work)
	// Output:
	// {engineer 10000}
}

func ExampleAtomicLoad() {
	engine, err := FromFile("./etc/config.yaml", WithCustomEtcdClient(&clientv3.Client{
		KV:      &testKV{},
		Watcher: &testWatcher{},
	}))
	if err != nil {
		panic(err)
	}

	var c workConfig
	err = AtomicStore(context.Background(), engine, "test/demo1", "work", &c.work, json.Unmarshal)
	if err != nil {
		panic(err)
	}

	w := AtomicLoad(&c.work)
	fmt.Println(*w)
	// Output:
	// {engineer 10000}
}

func TestAtomicLoad_NilField(t *testing.T) {
	var p *int
	testz.Nil(t, AtomicLoad(&p))
}
