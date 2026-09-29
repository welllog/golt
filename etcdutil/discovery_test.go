package etcdutil

import (
	"math"
	"testing"

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
	// d.n.Add(1) - 1 when d.n starts at math.MaxInt32:
	d.n.Store(uint32(math.MaxInt32))

	// The first call: d.n.Add(1)-1 = math.MaxInt32 (2147483647).
	// Next calls will produce values >= 2147483648 (which would be negative in 32-bit int).
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
	}
	empty := []string{}
	d.hosts.Store(&empty)

	_, err := d.Resolve()
	testz.Equal(t, true, err != nil)

	// Add host via PUT event
	d.Handle(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Value: []byte("127.0.0.1:9001"),
		},
	})

	testz.Equal(t, []string{"127.0.0.1:9001"}, d.ResolveAll())

	// Add second host
	d.Handle(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Value: []byte("127.0.0.1:9002"),
		},
	})

	testz.Equal(t, 2, len(d.ResolveAll()))

	// Delete host via DELETE event
	d.Handle(&clientv3.Event{
		Type: clientv3.EventTypeDelete,
		PrevKv: &mvccpb.KeyValue{
			Value: []byte("127.0.0.1:9001"),
		},
	})

	testz.Equal(t, []string{"127.0.0.1:9002"}, d.ResolveAll())

	// Invalid host should be ignored
	d.Handle(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Value: []byte("invalid-host-no-port"),
		},
	})
	testz.Equal(t, []string{"127.0.0.1:9002"}, d.ResolveAll())
}
