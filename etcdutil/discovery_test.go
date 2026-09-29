package etcdutil

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/welllog/golib/testz"
	"github.com/welllog/olog"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestDiscovery_Resolve32BitOverflow(t *testing.T) {
	d := &Discovery{
		service: "user-service",
		logger:  olog.DynamicLogger{},
	}
	hosts := []string{"127.0.0.1:8001", "127.0.0.1:8002", "127.0.0.1:8003"}
	d.hosts.Store(&hosts)

	// In 32-bit systems, values >= math.MaxInt32 + 1 would turn negative if converted to int before modulo.
	d.n.Store(uint32(math.MaxInt32))

	for i := 0; i < 10; i++ {
		host, err := d.Resolve()
		testz.Nil(t, err)
		testz.Equal(t, true, host == "127.0.0.1:8001" || host == "127.0.0.1:8002" || host == "127.0.0.1:8003")
	}

	// Test boundary near uint32 max
	d.n.Store(^uint32(0) - 2)
	for i := 0; i < 10; i++ {
		host, err := d.Resolve()
		testz.Nil(t, err)
		testz.Equal(t, true, host == "127.0.0.1:8001" || host == "127.0.0.1:8002" || host == "127.0.0.1:8003")
	}
}

func TestDiscovery_HandleEvents(t *testing.T) {
	d := &Discovery{
		service: "order-service",
		logger:  olog.DynamicLogger{},
		entries: make(map[string]string),
	}
	empty := []string{}
	d.hosts.Store(&empty)

	_, err := d.Resolve()
	testz.Equal(t, true, err != nil)

	// Add host 1
	d.Handle(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("order-service/inst1"),
			Value: []byte("127.0.0.1:9001"),
		},
	})
	testz.Equal(t, []string{"127.0.0.1:9001"}, d.ResolveAll())

	// Add host 2
	d.Handle(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("order-service/inst2"),
			Value: []byte("127.0.0.1:9002"),
		},
	})
	testz.Equal(t, 2, len(d.ResolveAll()))

	// Delete host 1 even when PrevKv is nil
	d.Handle(&clientv3.Event{
		Type: clientv3.EventTypeDelete,
		Kv: &mvccpb.KeyValue{
			Key: []byte("order-service/inst1"),
		},
		PrevKv: nil, // prevKv missing must not prevent deletion
	})
	testz.Equal(t, []string{"127.0.0.1:9002"}, d.ResolveAll())

	// Invalid host should be ignored
	d.Handle(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("order-service/inst3"),
			Value: []byte("invalid-host-no-port"),
		},
	})
	testz.Equal(t, []string{"127.0.0.1:9002"}, d.ResolveAll())
}

func TestDiscovery_MultipleKeysSameHost(t *testing.T) {
	d := &Discovery{
		service: "multi-key-service",
		logger:  olog.DynamicLogger{},
		entries: make(map[string]string),
	}
	empty := []string{}
	d.hosts.Store(&empty)

	// Two distinct keys register the same endpoint (e.g. graceful restart/overlap)
	d.Handle(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("multi-key-service/k1"),
			Value: []byte("10.0.0.1:8080"),
		},
	})
	d.Handle(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("multi-key-service/k2"),
			Value: []byte("10.0.0.1:8080"),
		},
	})

	// Hosts should be deduplicated
	testz.Equal(t, []string{"10.0.0.1:8080"}, d.ResolveAll())

	// Deleting the first key must NOT remove the host because k2 is still active!
	d.Handle(&clientv3.Event{
		Type: clientv3.EventTypeDelete,
		Kv: &mvccpb.KeyValue{
			Key: []byte("multi-key-service/k1"),
		},
	})
	testz.Equal(t, []string{"10.0.0.1:8080"}, d.ResolveAll())

	// Only when k2 is also deleted should the endpoint disappear
	d.Handle(&clientv3.Event{
		Type: clientv3.EventTypeDelete,
		Kv: &mvccpb.KeyValue{
			Key: []byte("multi-key-service/k2"),
		},
	})
	testz.Equal(t, 0, len(d.ResolveAll()))
}

func TestDiscovery_NewDiscoveryWithWatch(t *testing.T) {
	tkv := &testKV{}
	twt := &testWatcher{}
	c := clientv3.Client{
		KV:      tkv,
		Watcher: twt,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initial endpoint in KV
	_, _ = tkv.Put(ctx, "live-svc/node1", "127.0.0.1:7001")

	d, err := NewDiscoveryWithWatch(ctx, &c, "live-svc", nil)
	testz.Nil(t, err)
	testz.Equal(t, []string{"127.0.0.1:7001"}, d.ResolveAll())

	// Dynamic PUT via Watcher
	twt.notifyCreate("live-svc/node2", "127.0.0.1:7002")
	time.Sleep(10 * time.Millisecond)
	testz.Equal(t, 2, len(d.ResolveAll()))

	// Dynamic DELETE via Watcher
	twt.notifyDel("live-svc/node1")
	time.Sleep(10 * time.Millisecond)
	testz.Equal(t, []string{"127.0.0.1:7002"}, d.ResolveAll())
}

func TestDiscovery_ConcurrentReadSequentialWrite(t *testing.T) {
	d := &Discovery{
		service: "concurrent-svc",
		logger:  olog.DynamicLogger{},
		entries: make(map[string]string),
	}
	empty := []string{}
	d.hosts.Store(&empty)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Writer goroutine simulating the single sequential Watch stream
	go func() {
		i := 0
		for ctx.Err() == nil {
			inst := fmt.Sprintf("concurrent-svc/node%d", i%5)
			host := fmt.Sprintf("127.0.0.1:%d", 8000+i%5)
			if i%2 == 0 {
				d.Handle(&clientv3.Event{
					Type: clientv3.EventTypePut,
					Kv:   &mvccpb.KeyValue{Key: []byte(inst), Value: []byte(host)},
				})
			} else {
				d.Handle(&clientv3.Event{
					Type: clientv3.EventTypeDelete,
					Kv:   &mvccpb.KeyValue{Key: []byte(inst)},
				})
			}
			i++
		}
	}()

	// Multiple concurrent readers calling Resolve and ResolveAll
	for r := 0; r < 4; r++ {
		go func() {
			for ctx.Err() == nil {
				_, _ = d.Resolve()
				_ = d.ResolveAll()
			}
		}()
	}

	<-ctx.Done()
}
