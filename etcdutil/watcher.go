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

func (w *Watcher) SetCommonPrefixMinLen(l int) *Watcher {
	if l > 1 {
		w.commonPrefixMinLen = l
	}

	return w
}

func (w *Watcher) SetLogger(logger contract.Logger) *Watcher {
	w.logger = logger
	return w
}

// Attach not goroutine safe
func (w *Watcher) Attach(observer Observer) {
	prefix := observer.Prefix()
	var hasCommonPrefix bool
	for i, v := range w.prefixes {
		cpx := commonPrefix(prefix, v, w.commonPrefixMinLen)
		if cpx != "" {
			w.prefixes[i] = cpx
			hasCommonPrefix = true
			break
		}
	}

	if !hasCommonPrefix {
		w.prefixes = append(w.prefixes, prefix)
	}
	w.observers = append(w.observers, observer)
}

// HasObserver not goroutine safe
func (w *Watcher) HasObserver(prefix string) bool {
	for _, v := range w.observers {
		if v.Prefix() == prefix {
			return true
		}
	}
	return false
}

// Run should exec after Attach
func (w *Watcher) Run(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&w.state, 0, 1) {
		return
	}

	for _, v := range w.prefixes {
		prefix := v

		w.wg.Add(1)
		go w.watch(ctx, prefix)
	}

	w.wg.Wait()
}

func (w *Watcher) watch(ctx context.Context, prefix string) {
	ch := w.client.Watch(ctx, prefix, clientv3.WithPrefix(), clientv3.WithPrevKV())
	w.logger.Debugf("watch etcd key prefix: %s", prefix)

	w.wg.Done()

	for ret := range ch {
		for _, ev := range ret.Events {
			if ev.Type != clientv3.EventTypePut && ev.Type != clientv3.EventTypeDelete {
				continue
			}

			key := strz.UnsafeString(ev.Kv.Key)
			w.logger.Debugf("key %s %s", key, ev.Type.String())
			for _, obs := range w.observers {
				if strings.HasPrefix(key, obs.Prefix()) {
					w.logger.Debugf("key %s %s", key, obs.Prefix())
					func() {
						defer func() {
							if r := recover(); r != nil {
								w.logger.Errorf("observer.Handle panic: %v", r)
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
