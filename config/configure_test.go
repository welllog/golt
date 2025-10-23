package config

import (
	"bytes"
	"context"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/welllog/golt/config/driver"
	"github.com/welllog/golt/config/driver/etcd"
	"github.com/welllog/golt/config/meta"
	"github.com/welllog/golt/contract"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/welllog/golib/testz"
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

func initConfigure(t *testing.T) *Configure {
	tkv := testKV{}
	ctx := context.Background()
	tkv.Put(ctx, "/v1/test/demo4/foo", "demo1")
	tkv.Put(ctx, "/v1/test/demo4/bar", "demo2")
	tkv.Put(ctx, "/v1/test/demo5/baz", "")
	tkv.Put(ctx, "/v1/test/demo5/work", `{"title":"engineer","salary":10000}`)

	twt := testWatcher{}
	c := clientv3.Client{
		KV:      &tkv,
		Watcher: &twt,
	}

	driver.RegisterDriver("etcd", func(config meta.Config, logger contract.Logger) (driver.Driver, error) {
		return etcd.NewAdvanced(config, logger, etcd.WithCustomEtcdClient(&c))
	})

	engine, err := FromFile("./etc/config.yaml")
	testz.Nil(t, err)
	return engine
}

func TestConfigure_String(t *testing.T) {
	engine := initConfigure(t)

	ctx := context.Background()
	name1, err := engine.String(ctx, "test/demo1", "name")
	testz.Nil(t, err)

	name2, err := engine.String(ctx, "test/demo2", "name")
	testz.Nil(t, err)

	testz.Equal(t, "demo1", name1)
	testz.Equal(t, name1, name2)

	str, err := engine.String(ctx, "test/demo4", "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", str)

	str, err = engine.String(ctx, "test/demo5", "baz")
	testz.Nil(t, err)
	testz.Equal(t, "", str)
}

func TestConfigure_String_Empty(t *testing.T) {
	engine := initConfigure(t)

	ctx := context.Background()
	s1, err := engine.String(ctx, "test/demo1", "test_title")
	testz.Nil(t, err)
	testz.Equal(t, "", s1)

	s2, err := engine.String(ctx, "test/demo1", "no_value")
	testz.Nil(t, err)
	testz.Equal(t, "", s2)

	s3, err := engine.String(ctx, "test/demo3", "no_value")
	testz.Nil(t, err)
	testz.Equal(t, "", s3)

	s4, err := engine.String(ctx, "test/demo2", "test_name")
	testz.Nil(t, err)
	testz.Equal(t, "", s4)

	s5, err := engine.String(ctx, "test/demo3", "test_name")
	testz.Nil(t, err)
	testz.Equal(t, "", s5)
}

func TestConfigure_Int64(t *testing.T) {
	engine := initConfigure(t)

	ctx := context.Background()
	no, err := engine.Int64(ctx, "test/demo1", "no")
	testz.Nil(t, err)
	testz.Equal(t, int64(2), no)

	maxValue, err := engine.Int64(ctx, "test/demo3", "max_value")
	testz.Nil(t, err)
	testz.Equal(t, int64(120), maxValue)
}

func TestConfigure_Float64(t *testing.T) {
	engine := initConfigure(t)

	ctx := context.Background()
	testValue, err := engine.Float64(ctx, "test/demo1", "test_value")
	testz.Nil(t, err)
	testz.Equal(t, 12.3, testValue)

	testValue, err = engine.Float64(ctx, "test/demo3", "test_value")
	testz.Nil(t, err)
	testz.Equal(t, 12.3, testValue)
}

func TestConfigure_Bool(t *testing.T) {
	engine := initConfigure(t)

	ctx := context.Background()
	b1, err := engine.Bool(ctx, "test/demo1", "leader")
	testz.Nil(t, err)
	testz.Equal(t, true, b1)

	b2, err := engine.Bool(ctx, "test/demo2", "leader")
	testz.Nil(t, err)
	testz.Equal(t, b1, b2)
}

func TestConfigure_YamlDecode(t *testing.T) {
	engine := initConfigure(t)
	ctx := context.Background()

	var a addr
	err := engine.YamlDecode(ctx, "test/demo1", "addr", &a)
	testz.Nil(t, err)

	testz.Equal(t, "sichuan", a.Province)
	testz.Equal(t, "chengdu", a.City)
	testz.Equal(t, "nanjing", a.Street)
}

func TestConfigure_JsonDecode(t *testing.T) {
	engine := initConfigure(t)
	ctx := context.Background()

	var w work
	err := engine.JsonDecode(ctx, "test/demo1", "work", &w)
	testz.Nil(t, err)

	testz.Equal(t, "engineer", w.Title)
	testz.Equal(t, 10000, w.Salary)

	var w1 work
	err = engine.JsonDecode(ctx, "test/demo5", "work", &w1)
	testz.Nil(t, err)

	testz.Equal(t, "engineer", w1.Title)
	testz.Equal(t, 10000, w1.Salary)
}

func TestConfigureWatch(t *testing.T) {
	ctx := context.Background()
	f, err := os.OpenFile("./etc/test1.yaml", os.O_RDWR, 0666)
	testz.Nil(t, err)
	defer f.Close()

	b, err := io.ReadAll(f)
	testz.Nil(t, err)

	engine := initConfigure(t)

	var num int32
	engine.OnKeyChange("test/demo1", "name", func(b []byte) error {
		num++
		return nil
	})

	name, err := engine.String(ctx, "test/demo1", "name")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", name)

	_, _ = f.Seek(io.SeekStart, 0)
	_, _ = f.Write(b)
	time.Sleep(800 * time.Millisecond)
	name, err = engine.String(ctx, "test/demo1", "name")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", name)

	b2 := bytes.Replace(b, []byte("demo1"), []byte("demo2"), 1)
	_, err = f.Seek(io.SeekStart, 0)
	testz.Nil(t, err)
	_, err = f.Write(b2)
	testz.Nil(t, err)
	time.Sleep(800 * time.Millisecond)
	name, err = engine.String(ctx, "test/demo1", "name")
	testz.Nil(t, err)
	testz.Equal(t, "demo2", name)

	_, _ = f.Seek(io.SeekStart, 0)
	_, _ = f.Write(b)
	time.Sleep(800 * time.Millisecond)
	name, err = engine.String(ctx, "test/demo1", "name")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", name)

	testz.Equal(t, int32(2), num, "change event should be triggered twice")
}

type testKV struct {
	kvs []*kv
	fn  func(string)
}

type kv struct {
	key   string
	value string
}

func (t *testKV) Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	for _, v := range t.kvs {
		if v.key == key {
			v.value = val
			return &clientv3.PutResponse{}, nil
		}
	}

	t.kvs = append(t.kvs, &kv{key: key, value: val})
	slices.SortStableFunc(t.kvs, func(e1, e2 *kv) int {
		return strings.Compare(e1.key, e2.key)
	})
	return &clientv3.PutResponse{}, nil
}

func (t *testKV) SetGetHook(fn func(key string)) {
	t.fn = fn
}

func (t *testKV) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	var ops clientv3.Op
	for _, op := range opts {
		op(&ops)
	}

	if t.fn != nil {
		t.fn(key)
	}

	var ret clientv3.GetResponse
	if len(ops.RangeBytes()) > 0 {
		for _, v := range t.kvs {
			if strings.HasPrefix(v.key, key) {
				ret.Kvs = append(ret.Kvs, &mvccpb.KeyValue{
					Key:   []byte(v.key),
					Value: []byte(v.value),
				})
			}
		}
		return &ret, nil
	}

	for _, v := range t.kvs {
		if v.key == key {
			ret.Kvs = append(ret.Kvs, &mvccpb.KeyValue{
				Key:   []byte(v.key),
				Value: []byte(v.value),
			})
		}
	}
	return &ret, nil
}

func (t *testKV) Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	for i, v := range t.kvs {
		if v.key == key {
			t.kvs = append(t.kvs[:i], t.kvs[i+1:]...)
			return &clientv3.DeleteResponse{}, nil
		}
	}
	return &clientv3.DeleteResponse{}, nil
}

func (t *testKV) Compact(ctx context.Context, rev int64, opts ...clientv3.CompactOption) (*clientv3.CompactResponse, error) {
	panic("implement me")
}

func (t *testKV) Do(ctx context.Context, op clientv3.Op) (clientv3.OpResponse, error) {
	panic("implement me")
}

func (t *testKV) Txn(ctx context.Context) clientv3.Txn {
	panic("implement me")
}

type testWatcher struct {
	chs []*wch
}

type wch struct {
	key string
	ch  chan clientv3.WatchResponse
}

func (t *testWatcher) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan {
	w := wch{
		key: key,
		ch:  make(chan clientv3.WatchResponse, 10),
	}
	t.chs = append(t.chs, &w)
	return w.ch
}

func (t *testWatcher) notifyCreate(key, value string) {
	wrsp := clientv3.WatchResponse{
		Events: []*clientv3.Event{
			{
				Type: mvccpb.PUT,
				Kv: &mvccpb.KeyValue{
					Key:   []byte(key),
					Value: []byte(value),
				},
			},
		},
	}

	for _, v := range t.chs {
		if strings.HasPrefix(key, v.key) {
			v.ch <- wrsp
		}
	}
}

func (t *testWatcher) notifyDel(key string) {
	wrsp := clientv3.WatchResponse{
		Events: []*clientv3.Event{
			{
				Type: mvccpb.DELETE,
				Kv: &mvccpb.KeyValue{
					Key: []byte(key),
				},
			},
		},
	}

	for _, v := range t.chs {
		if strings.HasPrefix(key, v.key) {
			v.ch <- wrsp
		}
	}
}

func (t *testWatcher) RequestProgress(ctx context.Context) error {
	panic("implement me")
}

func (t *testWatcher) Close() error {
	return nil
}
