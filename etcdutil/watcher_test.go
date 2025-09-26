package etcdutil

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/welllog/golib/testz"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestKv_Handle(t *testing.T) {
	tkv := initTestKv()
	twt := testWatcher{}
	c := clientv3.Client{
		KV:      tkv,
		Watcher: &twt,
	}

	kv := NewKv("/v1/", &c)
	watcher := NewWatcher(&c)
	watcher.Attach(kv)
	ctx := context.Background()
	watcher.Run(ctx)

	val, err := kv.GetString(ctx, "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo1", val)

	val, err = kv.GetString(ctx, "bar")
	testz.Nil(t, err)
	testz.Equal(t, "demo2", val)

	var eventNum int
	kv.OnKeyChange("foo", func(b []byte) error {
		eventNum++
		return nil
	})
	twt.notifyCreate("/v1/foo", "demo10")
	time.Sleep(time.Millisecond)
	val, err = kv.GetString(ctx, "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo10", val)
	testz.Equal(t, 1, eventNum)

	twt.notifyCreate("/v1/foo", "demo10")
	time.Sleep(time.Millisecond)
	val, err = kv.GetString(ctx, "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo10", val)
	testz.Equal(t, 1, eventNum)

	twt.notifyDel("/v1/foo")
	time.Sleep(time.Millisecond)
	_, err = kv.GetString(ctx, "foo")
	testz.Equal(t, ErrNotFound, err)
}

func TestWatcher_SetCommonPrefixMinLen(t *testing.T) {
	twt := testWatcher{}
	c := clientv3.Client{
		Watcher: &twt,
	}

	watcher := NewWatcher(&c)
	watcher.SetCommonPrefixMinLen(2)
	kv1 := NewKv("/v1/", &c)
	kv2 := NewKv("/v2/", &c)
	watcher.Attach(kv1)
	watcher.Attach(kv2)

	testz.Equal(t, 1, len(watcher.prefixes))
	testz.Equal(t, "/v", watcher.prefixes[0])

	kv3 := NewKv("/t3/", &c)
	watcher.Attach(kv3)
	testz.Equal(t, 2, len(watcher.prefixes))
	testz.Equal(t, "/v", watcher.prefixes[0])
	testz.Equal(t, "/t3/", watcher.prefixes[1])

	kv4 := NewKv("/t4/", &c)
	watcher.Attach(kv4)
	testz.Equal(t, 2, len(watcher.prefixes))
	testz.Equal(t, "/v", watcher.prefixes[0])
	testz.Equal(t, "/t", watcher.prefixes[1])
}

type testWatcher struct {
	chs []*wch
	mu  sync.RWMutex
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
	t.mu.Lock()
	t.chs = append(t.chs, &w)
	t.mu.Unlock()
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

	t.mu.RLock()
	fmt.Printf("notify create %s %s, watch chann num: %d \n", key, value, len(t.chs))
	for _, v := range t.chs {
		if strings.HasPrefix(key, v.key) {
			v.ch <- wrsp
		}
	}
	t.mu.RUnlock()
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
