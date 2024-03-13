package etcdutil

import (
	"context"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type testWatcher struct {
}

type ch struct {
	key string
	ch  clientv3.WatchChan
}

func (t *testWatcher) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan {
	//TODO implement me
	panic("implement me")
}

func (t *testWatcher) notify(key string) {

}

func (t *testWatcher) RequestProgress(ctx context.Context) error {
	panic("implement me")
}

func (t *testWatcher) Close() error {
	return nil
}
