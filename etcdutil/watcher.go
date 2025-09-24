package etcdutil

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/welllog/golib/strz"
	"github.com/welllog/golt/contract"
	"github.com/welllog/olog"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Observer interface {
	Prefix() string
	Handle(event *clientv3.Event)
}

type Watcher struct {
	client             *clientv3.Client
	observers          []Observer
	prefixes           []string
	commonPrefixMinLen int
	state              int32
	logger             contract.Logger
	wg                 sync.WaitGroup
}

func NewWatcher(client *clientv3.Client) *Watcher {
	return &Watcher{
		client:             client,
		commonPrefixMinLen: 1,
		logger:             olog.DynamicLogger{},
	}
}

func (e *Watcher) SetCommonPrefixMinLen(l int) *Watcher {
	if l > 1 {
		e.commonPrefixMinLen = l
	}

	return e
}

func (e *Watcher) SetLogger(logger contract.Logger) *Watcher {
	e.logger = logger
	return e
}

// Attach not goroutine safe
func (e *Watcher) Attach(observer Observer) {
	prefix := observer.Prefix()
	var hasCommonPrefix bool
	for i, v := range e.prefixes {
		cpx := commonPrefix(prefix, v, e.commonPrefixMinLen)
		if cpx != "" {
			e.prefixes[i] = cpx
			hasCommonPrefix = true
			break
		}
	}

	if !hasCommonPrefix {
		e.prefixes = append(e.prefixes, prefix)
	}
	e.observers = append(e.observers, observer)
}

// HasObserver not goroutine safe
func (e *Watcher) HasObserver(prefix string) bool {
	for _, v := range e.observers {
		if v.Prefix() == prefix {
			return true
		}
	}
	return false
}

// Run should exec after Attach
func (e *Watcher) Run(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&e.state, 0, 1) {
		return
	}

	for _, v := range e.prefixes {
		prefix := v

		e.wg.Add(1)
		go e.watch(ctx, prefix)
	}

	e.wg.Wait()
}

func (e *Watcher) watch(ctx context.Context, prefix string) {
	ch := e.client.Watch(ctx, prefix, clientv3.WithPrefix(), clientv3.WithPrevKV())
	e.logger.Debugf("watch etcd key prefix: %s", prefix)

	e.wg.Done()

	for ret := range ch {
		for _, ev := range ret.Events {
			if ev.Type != clientv3.EventTypePut && ev.Type != clientv3.EventTypeDelete {
				continue
			}

			key := strz.UnsafeString(ev.Kv.Key)
			e.logger.Debugf("key %s %s", key, ev.Type.String())
			for _, obs := range e.observers {
				if strings.HasPrefix(key, obs.Prefix()) {
					e.logger.Debugf("key %s %s", key, obs.Prefix())
					func() {
						defer func() {
							if r := recover(); r != nil {
								e.logger.Errorf("observer.Handle panic: %v", r)
							}
						}()
						obs.Handle(ev)
					}()
				}
			}
		}
	}
}

func commonPrefix(s1, s2 string, commonSize int) string {
	var prefix string
	for i := 0; ; i++ {
		if i >= len(s1) || i >= len(s2) {
			prefix = s1[:i]
			break
		}

		if s1[i] != s2[i] {
			prefix = s1[:i]
			break
		}
	}

	if len(prefix) < commonSize {
		return ""
	}

	return prefix
}
