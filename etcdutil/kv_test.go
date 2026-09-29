package etcdutil

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

	// not watched: a miss is not cached, every Get hits etcd
	kv := NewKv("/v1/", &c)
	ctx := context.Background()
	_, err := kv.Get(ctx, "notfound")
	testz.Equal(t, ErrNotFound, err)
	testz.Equal(t, 1, getFromEtcdCount, "query should from etcd")

	_, err = kv.Get(ctx, "notfound")
	testz.Equal(t, ErrNotFound, err)
	testz.Equal(t, 2, getFromEtcdCount, "miss should not be cached when not watched")

	// watched: a miss is cached, no extra etcd round trip
	twt := testWatcher{}
	c2 := clientv3.Client{KV: tkv, Watcher: &twt}
	kv2 := NewKv("/v1/", &c2).SetWatched()
	watcher := NewWatcher(&c2)
	watcher.Attach(kv2)
	watcher.Run(context.Background())

	_, err = kv2.Get(ctx, "notfound")
	testz.Equal(t, ErrNotFound, err)
	testz.Equal(t, 3, getFromEtcdCount)

	_, err = kv2.Get(ctx, "notfound")
	testz.Equal(t, ErrNotFound, err)
	testz.Equal(t, 3, getFromEtcdCount, "miss should be cached when watched")
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

func TestKv_NegativeCacheWithoutWatch(t *testing.T) {
	tkv := initTestKv()
	c := clientv3.Client{KV: tkv}

	// not watched: a miss must not be cached, otherwise a later Put would
	// never become visible
	kv := NewKv("/v1/", &c)
	ctx := context.Background()

	_, err := kv.Get(ctx, "later")
	testz.Equal(t, ErrNotFound, err)

	tkv.Put(ctx, "/v1/later", "late")
	val, err := kv.Get(ctx, "later")
	testz.Nil(t, err)
	testz.Equal(t, "late", string(val))
}

func TestKv_NegativeCacheWithWatch(t *testing.T) {
	tkv := initTestKv()
	twt := testWatcher{}
	c := clientv3.Client{KV: tkv, Watcher: &twt}

	kv := NewKv("/v1/", &c).SetWatched()
	watcher := NewWatcher(&c)
	watcher.Attach(kv)
	watcher.Run(context.Background())

	ctx := context.Background()

	_, err := kv.Get(ctx, "later")
	testz.Equal(t, ErrNotFound, err)

	var getFromEtcdCount int
	tkv.SetGetHook(func(key string) { getFromEtcdCount++ })

	// watched: the miss is cached, no extra etcd round trip
	_, err = kv.Get(ctx, "later")
	testz.Equal(t, ErrNotFound, err)
	testz.Equal(t, 0, getFromEtcdCount)

	// ... and a watch event flips it positive
	twt.notifyCreate("/v1/later", "late")
	time.Sleep(time.Millisecond)
	val, err := kv.Get(ctx, "later")
	testz.Nil(t, err)
	testz.Equal(t, "late", string(val))
}

func TestKv_HandleHookMayCallOnKeyChange(t *testing.T) {
	tkv := initTestKv()
	twt := testWatcher{}
	c := clientv3.Client{KV: tkv, Watcher: &twt}

	kv := NewKv("/v1/", &c)
	watcher := NewWatcher(&c)
	watcher.Attach(kv)
	watcher.Run(context.Background())

	_, err := kv.GetString(context.Background(), "foo")
	testz.Nil(t, err)

	completed := make(chan struct{})
	kv.OnKeyChange("foo", func(b []byte) error {
		// used to deadlock: Handle executed hooks while holding the read lock
		kv.OnKeyChange("foo", func(b []byte) error { return nil })
		close(completed)
		return nil
	})

	twt.notifyCreate("/v1/foo", "demo10")

	select {
	case <-completed:
	case <-time.After(3 * time.Second):
		t.Fatal("hook calling OnKeyChange deadlocked")
	}
}

func TestKv_OnKeyChangeWithoutPriorGet(t *testing.T) {
	tkv := initTestKv()
	twt := testWatcher{}
	c := clientv3.Client{KV: tkv, Watcher: &twt}

	kv := NewKv("/v1/", &c)
	watcher := NewWatcher(&c)
	watcher.Attach(kv)
	watcher.Run(context.Background())

	// Register OnKeyChange WITHOUT calling Get first
	var received atomic.Pointer[string]
	kv.OnKeyChange("never_read", func(b []byte) error {
		s := string(b)
		received.Store(&s)
		return nil
	})

	// etcd receives a new PUT event
	twt.notifyCreate("/v1/never_read", "fresh_val")
	time.Sleep(10 * time.Millisecond)

	val := received.Load()
	if val == nil || *val != "fresh_val" {
		t.Fatalf("hook should be called on PUT even without prior Get, got %v", val)
	}

	// Subsequent Get should hit the newly cached entry
	got, err := kv.GetString(context.Background(), "never_read")
	testz.Nil(t, err)
	testz.Equal(t, "fresh_val", got)
}

func TestKv_OnKeyChangeWithPrevKvDiff(t *testing.T) {
	tkv := initTestKv()
	twt := testWatcher{}
	c := clientv3.Client{KV: tkv, Watcher: &twt}

	kv := NewKv("/v1/", &c)
	watcher := NewWatcher(&c)
	watcher.Attach(kv)
	watcher.Run(context.Background())

	var fireCount atomic.Int32
	kv.OnKeyChange("test_diff", func(b []byte) error {
		fireCount.Add(1)
		return nil
	})

	// Case 1: Identical value rewrite (PrevKv == Kv). Hook must NOT fire!
	twt.notifyPutWithPrevKv("/v1/test_diff", "same_val", "same_val")
	time.Sleep(10 * time.Millisecond)
	testz.Equal(t, int32(0), fireCount.Load(), "identical PUT should not trigger hook")

	// Case 2: Value changed (PrevKv != Kv). Hook MUST fire!
	twt.notifyPutWithPrevKv("/v1/test_diff", "same_val", "new_val")
	time.Sleep(10 * time.Millisecond)
	testz.Equal(t, int32(1), fireCount.Load(), "modified PUT must trigger hook")

	// Case 3: Another identical rewrite after cached. Hook must NOT fire!
	twt.notifyPutWithPrevKv("/v1/test_diff", "new_val", "new_val")
	time.Sleep(10 * time.Millisecond)
	testz.Equal(t, int32(1), fireCount.Load(), "identical PUT when cached must not trigger hook")
}

func BenchmarkKv_Handle(b *testing.B) {
	tkv := initTestKv()
	twt := testWatcher{}
	c := clientv3.Client{KV: tkv, Watcher: &twt}

	kv := NewKv("/v1/", &c)
	watcher := NewWatcher(&c)
	watcher.Attach(kv)
	watcher.Run(context.Background())

	_, _ = kv.GetString(context.Background(), "foo")
	kv.OnKeyChange("foo", func(b []byte) error { return nil })

	newEvent := func(val string) *clientv3.Event {
		return &clientv3.Event{
			Type: mvccpb.PUT,
			Kv:   &mvccpb.KeyValue{Key: []byte("/v1/foo"), Value: []byte(val)},
		}
	}

	b.Run("same_value", func(b *testing.B) {
		ev := newEvent("demo1") // cached value: no diff, no hook
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			kv.Handle(ev)
		}
	})

	b.Run("changed_value", func(b *testing.B) {
		evA := newEvent("demo10")
		evB := newEvent("demo11")
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if i%2 == 0 {
				kv.Handle(evA)
			} else {
				kv.Handle(evB)
			}
		}
	})
}
