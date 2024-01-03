package etcdutil

import (
	"context"
	"strings"
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
	prefixs            []string
	commonPrefixMinLen int
	state              int32
	logger             contract.Logger
}

func NewWatcher(client *clientv3.Client) *Watcher {
	return &Watcher{
		client:             client,
		commonPrefixMinLen: 1,
		logger:             olog.GetLogger(),
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
	for i, v := range e.prefixs {
		cpx := commonPrefix(prefix, v, e.commonPrefixMinLen)
		if cpx != "" {
			e.prefixs[i] = cpx
			hasCommonPrefix = true
			break
		}
	}

	if !hasCommonPrefix {
		e.prefixs = append(e.prefixs, prefix)
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

	chs := make([]clientv3.WatchChan, len(e.prefixs))
	for i, v := range e.prefixs {
		chs[i] = e.client.Watch(ctx, v, clientv3.WithPrefix())
		e.logger.Debugf("watch etcd key prefix: %s", v)
	}

	for _, ch := range chs {
		go func(wch clientv3.WatchChan) {
			for ret := range wch {
				for _, ev := range ret.Events {
					if ev.Type != clientv3.EventTypePut && ev.Type != clientv3.EventTypeDelete {
						continue
					}

					key := strz.UnsafeString(ev.Kv.Key)
					e.logger.Debugf("key %s %s", key, ev.Type.String())
					for _, obs := range e.observers {
						if strings.HasPrefix(key, obs.Prefix()) {
							obs.Handle(ev)
						}
					}
				}
			}
		}(ch)
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
