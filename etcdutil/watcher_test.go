package etcdutil

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/welllog/golib/testz"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
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

	var eventNum atomic.Int32
	kv.OnKeyChange("foo", func(b []byte) error {
		eventNum.Add(1)
		return nil
	})
	twt.notifyCreate("/v1/foo", "demo10")
	time.Sleep(time.Millisecond)
	val, err = kv.GetString(ctx, "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo10", val)
	testz.Equal(t, int32(1), eventNum.Load())

	twt.notifyCreate("/v1/foo", "demo10")
	time.Sleep(time.Millisecond)
	val, err = kv.GetString(ctx, "foo")
	testz.Nil(t, err)
	testz.Equal(t, "demo10", val)
	testz.Equal(t, int32(1), eventNum.Load())

	twt.notifyDel("/v1/foo")
	time.Sleep(time.Millisecond)
	_, err = kv.GetString(ctx, "foo")
	testz.Equal(t, ErrNotFound, err)
}

func TestWatcher_RestartAfterError(t *testing.T) {
	tkv := initTestKv()
	twt := testWatcher{}
	c := clientv3.Client{KV: tkv, Watcher: &twt}

	kv := NewKv("/v1/", &c)
	watcher := NewWatcher(&c)
	watcher.Attach(kv)
	watcher.Run(context.Background())

	ctx := context.Background()
	_, err := kv.GetString(ctx, "foo")
	testz.Nil(t, err)

	var fired atomic.Int32
	kv.OnKeyChange("foo", func(b []byte) error { fired.Add(1); return nil })

	twt.notifyCreate("/v1/foo", "demo10")
	time.Sleep(time.Millisecond)
	testz.Equal(t, int32(1), fired.Load())

	// a compaction error kills the stream; the watcher must re-establish it
	twt.notifyCanceled("/v1/")

	deadline := time.Now().Add(5 * time.Second)
	for twt.channelCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if twt.channelCount() < 2 {
		t.Fatal("watch was not restarted after error")
	}

	twt.notifyCreate("/v1/foo", "demo11")
	time.Sleep(10 * time.Millisecond)
	testz.Equal(t, int32(2), fired.Load(), "events after a watch error must still be delivered")
}

func waitForChannels(t *testing.T, twt *testWatcher, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for twt.channelCount() < n && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if twt.channelCount() < n {
		t.Fatalf("watcher did not establish %d streams", n)
	}
}

func TestWatcher_ResumeRevision(t *testing.T) {
	tkv := initTestKv()
	twt := testWatcher{}
	c := clientv3.Client{KV: tkv, Watcher: &twt}

	kv := NewKv("/v1/", &c)
	watcher := NewWatcher(&c)
	watcher.Attach(kv)
	watcher.Run(context.Background())

	// rev 0 means no WithRev: an unset start_revision is "now" per the etcd
	// proto, so the initial watch must not replay history
	waitForChannels(t, &twt, 1)
	testz.Equal(t, int64(0), twt.channelRev(0))

	twt.notifyCreateAtRev("/v1/foo", "demo1", 5)
	time.Sleep(10 * time.Millisecond)

	// a plain cancel must resume after the last seen revision
	twt.notifyPlainCancel("/v1/")
	waitForChannels(t, &twt, 2)
	testz.Equal(t, int64(6), twt.channelRev(1))

	// compaction invalidates the resume point; fall back to "now"
	twt.notifyCanceled("/v1/")
	waitForChannels(t, &twt, 3)
	testz.Equal(t, int64(0), twt.channelRev(2))
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

func TestWatcher_CanceledContextExits(t *testing.T) {
	twt := testWatcher{}
	c := clientv3.Client{Watcher: &twt}

	watcher := NewWatcher(&c)
	watcher.Attach(NewKv("/v1/", &c))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// a done ctx makes clientv3 return an already-closed channel; the
	// goroutine must exit instead of reconnecting forever
	before := runtime.NumGoroutine()
	watcher.Run(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Fatalf("watch goroutine leaked: %d goroutines, want <= %d", n, before)
	}
}

func TestWatcher_ClientClosedExits(t *testing.T) {
	twt := testWatcher{}
	c := clientv3.Client{Watcher: &twt}

	watcher := NewWatcher(&c)
	watcher.Attach(NewKv("/v1/", &c))

	before := runtime.NumGoroutine()
	watcher.Run(context.Background())
	waitForChannels(t, &twt, 1)

	// a closed client makes every Watch return an already-closed channel;
	// the goroutine must exit instead of reconnecting forever
	twt.close()

	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Fatalf("watch goroutine leaked: %d goroutines, want <= %d", n, before)
	}
	// a spinning watcher would dial again every watchRestartInterval
	testz.Equal(t, 1, twt.channelCount())
}

type testWatcher struct {
	chs    []*wch
	closed bool
	mu     sync.RWMutex
}

type wch struct {
	key string
	ch  chan clientv3.WatchResponse
	rev int64 // requested start revision; 0 means "now"
}

func (t *testWatcher) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan {
	// mirror clientv3: a done ctx or a closed client yields a closed channel
	if ctx.Err() != nil || t.isClosed() {
		ch := make(chan clientv3.WatchResponse)
		close(ch)
		return ch
	}

	var op clientv3.Op
	for _, opt := range opts {
		opt(&op)
	}

	w := wch{
		key: key,
		ch:  make(chan clientv3.WatchResponse, 10),
		rev: op.Rev(),
	}
	t.mu.Lock()
	t.chs = append(t.chs, &w)
	t.mu.Unlock()
	return w.ch
}

func (t *testWatcher) isClosed() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.closed
}

// close simulates closing the etcd client: live channels are closed and
// every later Watch returns an already-closed channel.
func (t *testWatcher) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	for _, v := range t.chs {
		close(v.ch)
	}
}

func (t *testWatcher) notifyCreate(key, value string) {
	t.notifyCreateAtRev(key, value, 0)
}

func (t *testWatcher) notifyCreateAtRev(key, value string, rev int64) {
	wrsp := clientv3.WatchResponse{
		Header: etcdserverpb.ResponseHeader{Revision: rev},
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

func (t *testWatcher) notifyPutWithPrevKv(key, prevVal, newVal string) {
	wrsp := clientv3.WatchResponse{
		Events: []*clientv3.Event{
			{
				Type: mvccpb.PUT,
				PrevKv: &mvccpb.KeyValue{
					Key:   []byte(key),
					Value: []byte(prevVal),
				},
				Kv: &mvccpb.KeyValue{
					Key:   []byte(key),
					Value: []byte(newVal),
				},
			},
		},
	}

	t.mu.RLock()
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

// notifyCanceled sends a canceled (compacted) response, making Err() non-nil.
func (t *testWatcher) notifyCanceled(key string) {
	wrsp := clientv3.WatchResponse{
		Canceled:        true,
		CompactRevision: 1,
	}

	t.mu.RLock()
	for _, v := range t.chs {
		if strings.HasPrefix(key, v.key) {
			v.ch <- wrsp
		}
	}
	t.mu.RUnlock()
}

// notifyPlainCancel cancels the stream without compaction, so the watcher
// must resume from the last seen revision.
func (t *testWatcher) notifyPlainCancel(key string) {
	wrsp := clientv3.WatchResponse{Canceled: true}

	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, v := range t.chs {
		if strings.HasPrefix(key, v.key) {
			v.ch <- wrsp
		}
	}
}

func (t *testWatcher) channelCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.chs)
}

func (t *testWatcher) channelRev(i int) int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.chs[i].rev
}

func (t *testWatcher) RequestProgress(ctx context.Context) error {
	panic("implement me")
}

func (t *testWatcher) Close() error {
	return nil
}
