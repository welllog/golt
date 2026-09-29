package etcdutil

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/welllog/golib/testz"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type testLease struct {
	grantCount     atomic.Int32
	keepAliveCount atomic.Int32
	keepAliveCtxs  []context.Context
	mu             sync.Mutex
}

func (l *testLease) Grant(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
	id := clientv3.LeaseID(l.grantCount.Add(1))
	return &clientv3.LeaseGrantResponse{ID: id, TTL: ttl}, nil
}

func (l *testLease) Revoke(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error) {
	return &clientv3.LeaseRevokeResponse{}, nil
}

func (l *testLease) TimeToLive(ctx context.Context, id clientv3.LeaseID, opts ...clientv3.LeaseOption) (*clientv3.LeaseTimeToLiveResponse, error) {
	return nil, nil
}

func (l *testLease) Leases(ctx context.Context) (*clientv3.LeaseLeasesResponse, error) {
	return nil, nil
}

func (l *testLease) KeepAlive(ctx context.Context, id clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error) {
	l.keepAliveCount.Add(1)
	l.mu.Lock()
	l.keepAliveCtxs = append(l.keepAliveCtxs, ctx)
	l.mu.Unlock()

	ch := make(chan *clientv3.LeaseKeepAliveResponse)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func (l *testLease) KeepAliveOnce(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseKeepAliveResponse, error) {
	return nil, nil
}

func (l *testLease) Close() error {
	return nil
}

func TestRegistrar_RegisterAndDeregister(t *testing.T) {
	tkv := &testKV{}
	tl := &testLease{}
	c := clientv3.Client{
		KV:    tkv,
		Lease: tl,
	}

	reg := NewRegister(&c, RegistrarConfig{
		LeaseTTL:      10,
		RetryInterval: 50 * time.Millisecond,
		OpTimeout:     500 * time.Millisecond,
	})

	ctx := context.Background()
	key, err := reg.RegisterService(ctx, "my-service", 9000)
	testz.Nil(t, err)

	reg.mu.Lock()
	cancel, ok := reg.cancels[key]
	testz.Equal(t, true, ok)
	testz.Equal(t, true, cancel != nil)
	reg.mu.Unlock()

	testz.Equal(t, int32(1), tl.grantCount.Load())
	time.Sleep(20 * time.Millisecond)

	// Verify keepAlive context is active
	tl.mu.Lock()
	testz.Equal(t, 1, len(tl.keepAliveCtxs))
	kCtx := tl.keepAliveCtxs[0]
	tl.mu.Unlock()
	testz.Nil(t, kCtx.Err())

	// Now deregister
	err = reg.DeregisterService(key)
	testz.Nil(t, err)

	// Cancels map should be cleared
	reg.mu.Lock()
	_, exists := reg.cancels[key]
	testz.Equal(t, false, exists)
	reg.mu.Unlock()

	// Wait for keepAlive context to be canceled and goroutine to exit
	select {
	case <-kCtx.Done():
	case <-time.After(1 * time.Second):
		t.Fatal("keepAlive context was not canceled after DeregisterService")
	}

	// Wait past the retry interval to ensure it does not re-register ("resurrect")
	time.Sleep(100 * time.Millisecond)
	testz.Equal(t, int32(1), tl.grantCount.Load(), "should not grant a new lease after deregistration")
}

func TestGetOutboundIP(t *testing.T) {
	ip, err := GetOutboundIP()
	if err == nil {
		parsed := net.ParseIP(ip)
		testz.Equal(t, true, parsed != nil)
		testz.Equal(t, true, parsed.To4() != nil)
		testz.Equal(t, false, parsed.IsLoopback())
	}

	ip2, err := GetOutboundIP("8.8.8.8:80")
	if err == nil {
		testz.Equal(t, ip, ip2)
	}

	ip3, err := GetOutboundIP("http://8.8.8.8:80/path")
	if err == nil {
		testz.Equal(t, ip, ip3)
	}
}

func TestGetLocalIP(t *testing.T) {
	ip, err := GetLocalIP()
	if err == nil {
		parsed := net.ParseIP(ip)
		testz.Equal(t, true, parsed != nil)
		testz.Equal(t, true, parsed.To4() != nil)
		testz.Equal(t, false, parsed.IsLoopback())
	}
}

func TestIsVirtualInterface(t *testing.T) {
	testz.Equal(t, true, isVirtualInterface("docker0"))
	testz.Equal(t, true, isVirtualInterface("veth1234"))
	testz.Equal(t, true, isVirtualInterface("br-abcd"))
	testz.Equal(t, true, isVirtualInterface("cni0"))
	testz.Equal(t, true, isVirtualInterface("flannel.1"))
	testz.Equal(t, true, isVirtualInterface("calico123"))
	testz.Equal(t, true, isVirtualInterface("tun0"))
	testz.Equal(t, true, isVirtualInterface("utun3"))
	testz.Equal(t, true, isVirtualInterface("tap0"))
	testz.Equal(t, true, isVirtualInterface("wg0"))
	testz.Equal(t, false, isVirtualInterface("eth0"))
	testz.Equal(t, false, isVirtualInterface("en0"))
	testz.Equal(t, false, isVirtualInterface("wlan0"))
}

func TestRegistrar_GetServiceIP(t *testing.T) {
	// Case 1: ServiceIP explicitly specified
	r := &Registrar{
		config: RegistrarConfig{
			ServiceIP: "192.168.10.99",
		},
	}
	ip, err := r.getServiceIP()
	testz.Nil(t, err)
	testz.Equal(t, "192.168.10.99", ip)

	// Case 1b: invalid ServiceIP is rejected
	r = &Registrar{
		config: RegistrarConfig{
			ServiceIP: "not-an-ip",
		},
	}
	_, err = r.getServiceIP()
	testz.Equal(t, true, err != nil)

	r = &Registrar{
		config: RegistrarConfig{
			ServiceIP: "::1",
		},
	}
	_, err = r.getServiceIP()
	testz.Equal(t, true, err != nil)

	// Case 2: Non-existent IfaceName returns error
	r = &Registrar{
		config: RegistrarConfig{
			IfaceName: "non_existent_interface_xyz_999",
		},
	}
	_, err = r.getServiceIP()
	testz.Equal(t, true, err != nil)

	// Case 3: Loopback-only interface has no usable IP.
	// "lo0" is the macOS name, "lo" the Linux one; both must fail.
	for _, name := range []string{"lo0", "lo"} {
		r = &Registrar{
			config: RegistrarConfig{
				IfaceName: name,
			},
		}
		_, err = r.getServiceIP()
		testz.Equal(t, true, err != nil, "iface "+name)
	}

	// Case 4: Default fallback
	r = &Registrar{}
	ip, err = r.getServiceIP()
	if err == nil {
		parsed := net.ParseIP(ip)
		testz.Equal(t, true, parsed != nil)
		testz.Equal(t, true, parsed.To4() != nil)
		testz.Equal(t, false, parsed.IsLoopback())
	}
}
