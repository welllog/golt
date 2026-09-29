package etcdutil

import (
	"context"
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
