package etcdutil

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/welllog/golib/testz"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func initTestKv() *testKV {
	kv := testKV{}
	ctx := context.Background()
	kv.Put(ctx, "/v1/foo", "demo1")
	kv.Put(ctx, "/v1/bar", "demo2")
	kv.Put(ctx, "/v1/baz", "demo3")
	kv.Put(ctx, "/v1/baz/1", "demo4")
	kv.Put(ctx, "/v2/foo", "demo5")

	return &kv
}

func TestKv_Get(t *testing.T) {
	tkv := initTestKv()
	c := clientv3.Client{
		KV: tkv,
	}

	kv := NewKv("/v1/", &c)
	ctx := context.Background()
	val, err := kv.Get(ctx, "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", string(val))

	val, err = kv.Get(ctx, "bar")
	testz.Nil(t, err)
	testz.Equal(t, "demo2", string(val))

	val, err = kv.Get(ctx, "baz")
	testz.Nil(t, err)
	testz.Equal(t, "demo3", string(val))

	var getFromEtcdCount int
	tkv.SetGetHook(func(key string) {
		getFromEtcdCount++
	})

	val, err = kv.Get(ctx, "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", string(val))

	val, err = kv.Get(ctx, "bar")
	testz.Nil(t, err)
	testz.Equal(t, "demo2", string(val))

	testz.Equal(t, 0, getFromEtcdCount, "query should from cache")

	val, err = kv.Get(ctx, "baz/1")
	testz.Nil(t, err)
	testz.Equal(t, "demo4", string(val))
	testz.Equal(t, 1, getFromEtcdCount, "query should from etcd")

	val, err = kv.GetNoCache(ctx, "baz/1")
	testz.Nil(t, err)
	testz.Equal(t, "demo4", string(val))
	testz.Equal(t, 2, getFromEtcdCount, "query should from etcd")
}

func TestKv_UnsafeGet(t *testing.T) {
	tkv := initTestKv()
	c := clientv3.Client{
		KV: tkv,
	}

	kv := NewKv("/v1/", &c)
	ctx := context.Background()
	val, err := kv.UnsafeGet(ctx, "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", string(val))

	val, err = kv.UnsafeGet(ctx, "bar")
	testz.Nil(t, err)
	testz.Equal(t, "demo2", string(val))

	val, err = kv.UnsafeGet(ctx, "baz")
	testz.Nil(t, err)
	testz.Equal(t, "demo3", string(val))

	var getFromEtcdCount int
	tkv.SetGetHook(func(key string) {
		getFromEtcdCount++
	})

	val, err = kv.UnsafeGet(ctx, "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", string(val))

	val, err = kv.UnsafeGet(ctx, "bar")
	testz.Nil(t, err)
	testz.Equal(t, "demo2", string(val))

	testz.Equal(t, 0, getFromEtcdCount, "query should from cache")

	val, err = kv.UnsafeGet(ctx, "baz/1")
	testz.Nil(t, err)
	testz.Equal(t, "demo4", string(val))
	testz.Equal(t, 1, getFromEtcdCount, "query should from etcd")

	val, err = kv.GetNoCache(ctx, "baz/1")
	testz.Nil(t, err)
	testz.Equal(t, "demo4", string(val))
	testz.Equal(t, 2, getFromEtcdCount, "query should from etcd")
}

func TestKv_GetString(t *testing.T) {
	tkv := initTestKv()
	c := clientv3.Client{
		KV: tkv,
	}

	kv := NewKv("/v1/", &c)
	ctx := context.Background()
	val, err := kv.GetString(ctx, "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", val)

	val, err = kv.GetString(ctx, "bar")
	testz.Nil(t, err)
	testz.Equal(t, "demo2", val)

	val, err = kv.GetString(ctx, "baz")
	testz.Nil(t, err)
	testz.Equal(t, "demo3", val)

	var getFromEtcdCount int
	tkv.SetGetHook(func(key string) {
		getFromEtcdCount++
	})

	val, err = kv.GetString(ctx, "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", val)

	val, err = kv.GetString(ctx, "bar")
	testz.Nil(t, err)
	testz.Equal(t, "demo2", val)

	testz.Equal(t, 0, getFromEtcdCount, "query should from cache")

	val, err = kv.GetString(ctx, "baz/1")
	testz.Nil(t, err)
	testz.Equal(t, "demo4", val)
	testz.Equal(t, 1, getFromEtcdCount, "query should from etcd")
}

func TestKv_Preload(t *testing.T) {
	tkv := initTestKv()
	c := clientv3.Client{
		KV: tkv,
	}

	kv := NewKv("/v1/", &c)
	ctx := context.Background()
	err := kv.Preload(ctx)
	testz.Nil(t, err)

	var getFromEtcdCount int
	tkv.SetGetHook(func(key string) {
		getFromEtcdCount++
	})

	val, err := kv.GetString(ctx, "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", val)

	val, err = kv.GetString(ctx, "bar")
	testz.Nil(t, err)
	testz.Equal(t, "demo2", val)

	val, err = kv.GetString(ctx, "baz")
	testz.Nil(t, err)
	testz.Equal(t, "demo3", val)

	testz.Equal(t, 0, getFromEtcdCount, "query should from cache")
}

func TestKv_Get_NotFound(t *testing.T) {
	tkv := initTestKv()
	c := clientv3.Client{
		KV: tkv,
	}

	var getFromEtcdCount int
	tkv.SetGetHook(func(key string) {
		getFromEtcdCount++
	})

	kv := NewKv("/v1/", &c)
	ctx := context.Background()
	_, err := kv.Get(ctx, "notfound")
	testz.Equal(t, ErrNotFound, err)
	testz.Equal(t, 1, getFromEtcdCount, "query should from etcd")

	_, err = kv.Get(ctx, "notfound")
	testz.Equal(t, ErrNotFound, err)
	testz.Equal(t, 1, getFromEtcdCount, "query should from cache")
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
