package config

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/welllog/golt/config/driver"
	"github.com/welllog/golt/config/driver/etcd"
	"github.com/welllog/golt/config/meta"
	"github.com/welllog/golt/contract"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type workConfig struct {
	work *work
}

func ExampleAtomicStore() {
	driver.RegisterDriver("etcd", func(config meta.Config, logger contract.Logger) (driver.Driver, error) {
		return etcd.NewAdvanced(config, logger, etcd.WithCustomEtcdClient(&clientv3.Client{
			KV:      &testKV{},
			Watcher: &testWatcher{},
		}))
	})

	engine, err := FromFile("./etc/config.yaml")
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
	driver.RegisterDriver("etcd", func(config meta.Config, logger contract.Logger) (driver.Driver, error) {
		return etcd.NewAdvanced(config, logger, etcd.WithCustomEtcdClient(&clientv3.Client{
			KV:      &testKV{},
			Watcher: &testWatcher{},
		}))
	})

	engine, err := FromFile("./etc/config.yaml")
	if err != nil {
		panic(err)
	}

	var c workConfig
	err = AtomicStore(context.Background(), engine, "test/demo1", "work", &c.work, json.Unmarshal)
	if err != nil {
		panic(err)
	}

	w := (*work)(AtomicLoad(&c.work))
	fmt.Println(*w)
	// Output:
	// {engineer 10000}
}
