package etcdutil

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/welllog/golib/strz"
	"github.com/welllog/golt/contract"
	"github.com/welllog/olog"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// watchRestartInterval is the delay before re-establishing a broken watch stream.
const watchRestartInterval = time.Second

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
	if logger != nil {
		w.logger = logger
	}
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
	ch := w.watchFrom(ctx, prefix, 0)
	w.logger.Debugf("watch etcd key prefix: %s", prefix)

	w.wg.Done()

	// lastRev is the newest revision seen on the stream; resuming from it on
	// restart keeps events in the restart gap from being lost. 0 means "now".
	var lastRev int64

	for {
		// clientv3 never returns a nil channel: it returns a closed one when
		// the watch ctx is done or the client is closed, and delivers a
		// Canceled response (Err() != nil) before closing when the stream
		// dies. Transient network errors are reconnected internally and do
		// not close the channel at all.
		streamFailed := false
		for ret := range ch {
			// a canceled/compacted stream is dead: restart it below,
			// otherwise events would be silently dropped forever
			if err := ret.Err(); err != nil {
				w.logger.Warnf("watch prefix %s error: %v, restarting", prefix, err)
				streamFailed = true
				if ret.CompactRevision != 0 {
					// history below the compact revision is gone; resume from now
					lastRev = 0
				}
				break
			}

			if ret.Header.Revision > lastRev {
				lastRev = ret.Header.Revision
			}

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

		// the channel closed without an error response: clientv3 only does
		// this for a cancelled watch ctx or a closed client, and every Watch
		// on a closed client returns a closed channel again, so retrying
		// would spin forever
		if !streamFailed {
			w.logger.Warnf("watch etcd key prefix: %s stopped: channel closed", prefix)
			return
		}

		select {
		case <-ctx.Done():
			w.logger.Warnf("watch etcd key prefix: %s stopped", prefix)
			return
		case <-time.After(watchRestartInterval):
		}

		ch = w.watchFrom(ctx, prefix, lastRev)
		w.logger.Debugf("watch etcd key prefix: %s restarted", prefix)
	}
}

// watchFrom opens a prefix watch resuming after rev; rev <= 0 watches from now.
func (w *Watcher) watchFrom(ctx context.Context, prefix string, rev int64) clientv3.WatchChan {
	opts := make([]clientv3.OpOption, 0, 3)
	opts = append(opts, clientv3.WithPrefix(), clientv3.WithPrevKV())
	if rev > 0 {
		opts = append(opts, clientv3.WithRev(rev+1))
	}
	return w.client.Watch(ctx, prefix, opts...)
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

	if prefix == s1 || prefix == s2 {
		return prefix
	}

	if len(prefix) < commonSize {
		return ""
	}

	return prefix
}
