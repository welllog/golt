package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/welllog/golib/testz"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type configDemo struct {
	Addr     addrDemo  `config:"namespace:test/demo1;key:addr;format:yaml"`
	addr     addrDemo  `config:"namespace:test/demo1;key:addr;format:yaml"`
	addr2    *addrDemo `config:"namespace:test/demo1;key:addr;format:yaml"`
	addr3    *addrDemo `config:"namespace:test/demo1;key:addr;format:yaml;lazy:true"`
	addr4    *addrDemo `config:"namespace:test/demo1;key:addr;format:yaml;watch:true"`
	addr5    *addrDemo `config:"namespace:test/demo1;key:addr;format:yaml;lazy:true;watch:true"`
	name     string    `config:"namespace:test/demo1;key:name"`
	no       int       `config:"namespace:test/demo1;key:no"`
	notExist *string   `config:"namespace:test/demo1;key:notExist;lazy:true"`
}

type addrDemo struct {
	Province string `yaml:"province"`
	City     string `yaml:"city"`
	Street   string `yaml:"street"`
}

func TestConfigure_InitAndPreload(t *testing.T) {
	engine, err := FromFile("./etc/config2.yaml", WithCustomEtcdClient(&clientv3.Client{
		KV:      &testKV{},
		Watcher: &testWatcher{},
	}))
	if err != nil {
		panic(err)
	}

	var c configDemo
	funcs, err := engine.InitAndPreload(&c, time.Second)
	testz.Nil(t, err)

	province1 := "sichuan"
	testz.Equal(t, c.Addr.Province, province1)
	testz.Equal(t, c.addr.Province, province1)
	testz.Equal(t, (*c.addr2).Province, province1)
	if c.addr3 != nil {
		t.Errorf("addr3 should be nil")
	}
	testz.Equal(t, (*c.addr4).Province, province1)
	if c.addr5 != nil {
		t.Errorf("addr5 should be nil")
	}
	testz.Equal(t, c.name, "demo1")
	if c.name == "" {
		t.Errorf("name should not be empty")
	}
	testz.Equal(t, c.no, 2)

	valPtr, err := TryLoad(&c.addr3, funcs)
	testz.Nil(t, err)

	testz.Equal(t, (*c.addr3).Province, province1)
	testz.Equal(t, valPtr.Province, province1)

	province2 := "xichuan"
	f, err := os.OpenFile("./etc/test2.yaml", os.O_RDWR, 0666)
	testz.Nil(t, err)
	defer f.Close()

	b, err := io.ReadAll(f)
	testz.Nil(t, err)

	b2 := bytes.Replace(b, []byte(province1), []byte(province2), 1)
	_, err = f.Seek(io.SeekStart, 0)
	testz.Nil(t, err)
	_, err = f.Write(b2)
	testz.Nil(t, err)
	time.Sleep(600 * time.Millisecond)

	testz.Equal(t, c.Addr.Province, province1)
	testz.Equal(t, c.addr.Province, province1)
	testz.Equal(t, (*c.addr2).Province, province1)
	testz.Equal(t, (*c.addr3).Province, province1)
	testz.Equal(t, AtomicLoad(&c.addr4).Province, province2)
	testz.Equal(t, AtomicLoad(&c.addr5).Province, province2)

	_, err = f.Seek(io.SeekStart, 0)
	testz.Nil(t, err)
	_, err = f.Write(b)
	testz.Nil(t, err)

	_, err = TryLoad(&c.notExist, funcs)
	if err == nil {
		t.Errorf("notExist should be error")
	}
	fmt.Println(err)
}

func TestConfigure_InitAndPreloadZeroTimeout(t *testing.T) {
	// timeout <= 0 means no timeout; it used to expire the load context immediately
	engine, err := FromFile("./etc/config2.yaml", WithCustomEtcdClient(&clientv3.Client{
		KV:      &testKV{},
		Watcher: &testWatcher{},
	}))
	if err != nil {
		panic(err)
	}

	var c configDemo
	_, err = engine.InitAndPreload(&c, 0)
	testz.Nil(t, err)
	testz.Equal(t, "demo1", c.name)
}

func TestConfigure_InitAndPreload_ExportedFieldWatchError(t *testing.T) {
	engine, err := FromFile("./etc/config2.yaml", WithCustomEtcdClient(&clientv3.Client{
		KV:      &testKV{},
		Watcher: &testWatcher{},
	}))
	testz.Nil(t, err)

	type badExported struct {
		Addr *addrDemo `config:"namespace:test/demo1;key:addr;watch:true"`
	}

	var b badExported
	_, err = engine.InitAndPreload(&b, 0)
	if err == nil || !strings.Contains(err.Error(), "is exported: lazy and watch are only supported") {
		t.Fatalf("expected error for exported watch field, got: %v", err)
	}
}

func TestConfigure_InitAndPreload_NotPtrFieldWatchError(t *testing.T) {
	engine, err := FromFile("./etc/config2.yaml", WithCustomEtcdClient(&clientv3.Client{
		KV:      &testKV{},
		Watcher: &testWatcher{},
	}))
	testz.Nil(t, err)

	type badNotPtr struct {
		addr addrDemo `config:"namespace:test/demo1;key:addr;watch:true"`
	}

	var b badNotPtr
	_, err = engine.InitAndPreload(&b, 0)
	if err == nil || !strings.Contains(err.Error(), "is not a pointer: lazy and watch are only supported") {
		t.Fatalf("expected error for non-pointer watch field, got: %v", err)
	}
}
