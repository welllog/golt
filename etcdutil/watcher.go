package etcdutil

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/welllog/golib/strz"
	"go.etcd.io/etcd/client/v3"
)

var (
	_errNotInRootPath = errors.New("current path not in watcher root path")
	_errWatcherHasRun = errors.New("watcher has run")
)

type EtcdObserver interface {
	Prefix() string
	Handle(event *clientv3.Event)
}

type EtcdWatcher struct {
	client    *clientv3.Client
	rootPath  string
	observers []EtcdObserver
	state     int
	mu        sync.Mutex
}

func NewEtcdWatcher(client *clientv3.Client, rootPath string) *EtcdWatcher {
	return &EtcdWatcher{
		client:   client,
		rootPath: rootPath,
	}
}

func (e *EtcdWatcher) AttachObserver(observer EtcdObserver) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == 1 {
		return _errWatcherHasRun
	}

	path := observer.Prefix()
	if !strings.HasPrefix(path, e.rootPath) {
		return _errNotInRootPath
	}

	e.observers = append(e.observers, observer)
	return nil
}

func (e *EtcdWatcher) Run(ctx context.Context) {
	e.mu.Lock()
	if e.state == 1 {
		e.mu.Unlock()
		return
	}
	e.state = 1
	e.mu.Unlock()

	rch := e.client.Watch(ctx, e.rootPath, clientv3.WithPrefix())
	for rsp := range rch {
		for _, ev := range rsp.Events {
			key := strz.UnsafeString(ev.Kv.Key)
			for _, obs := range e.observers {
				if strings.HasPrefix(key, obs.Prefix()) {
					obs.Handle(ev)
				}
			}
		}
	}
}
