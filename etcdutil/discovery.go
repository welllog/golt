package etcdutil

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sync/atomic"
	"time"

	"github.com/welllog/golt/contract"
	"github.com/welllog/olog"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var _ Observer = (*Discovery)(nil)

type Discovery struct {
	service string
	hosts   atomic.Pointer[[]string]
	n       atomic.Uint32
	logger  contract.Logger
	entries map[string]string // key -> host, sequentially maintained by the watch goroutine
}

// NewDiscovery creates a new Discovery instance without watching for changes.
func NewDiscovery(etcd *clientv3.Client, serviceName string, logger contract.Logger) (*Discovery, error) {
	if logger == nil {
		logger = olog.DynamicLogger{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := etcd.Get(ctx, serviceName, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("[NewDiscovery] etcd get: %w", err)
	}

	d := &Discovery{
		service: serviceName,
		logger:  logger,
		entries: make(map[string]string, len(resp.Kvs)),
	}

	for _, kv := range resp.Kvs {
		v := string(kv.Value)
		_, _, err = net.SplitHostPort(v)
		if err != nil {
			logger.Warnf("[NewDiscovery] %s invalid host: %s, err: %v", serviceName, v, err)
			continue
		}

		d.entries[string(kv.Key)] = v
	}
	d.rebuildHosts()

	return d, nil
}

// NewDiscoveryWithWatch creates a new Discovery instance and starts watching for changes
// with automatic reconnection and revision resumption via Watcher.
func NewDiscoveryWithWatch(ctx context.Context, etcd *clientv3.Client, serviceName string, logger contract.Logger) (*Discovery, error) {
	if logger == nil {
		logger = olog.DynamicLogger{}
	}

	d, err := NewDiscovery(etcd, serviceName, logger)
	if err != nil {
		return nil, err
	}

	watcher := NewWatcher(etcd).SetLogger(logger)
	watcher.Attach(d)
	watcher.Run(ctx)

	return d, nil
}

func (d *Discovery) Resolve() (string, error) {
	p := d.hosts.Load()
	if len(*p) == 0 {
		return "", fmt.Errorf("%s no endpoints", d.service)
	}

	if len(*p) == 1 {
		return (*p)[0], nil
	} else {
		idx := int((d.n.Add(1) - 1) % uint32(len(*p)))
		return (*p)[idx], nil
	}
}

func (d *Discovery) ResolveAll() []string {
	p := d.hosts.Load()
	if len(*p) == 0 {
		return nil
	}

	hosts := make([]string, len(*p))
	copy(hosts, *p)
	return hosts
}

func (d *Discovery) Prefix() string {
	return d.service
}

func (d *Discovery) Handle(ev *clientv3.Event) {
	key := string(ev.Kv.Key)
	switch ev.Type {
	case clientv3.EventTypePut:
		host := string(ev.Kv.Value)
		if _, _, err := net.SplitHostPort(host); err != nil {
			d.logger.Warnf("[Handle] %s invalid host: %s, err: %v", d.service, host, err)
			return
		}

		if old, ok := d.entries[key]; ok && old == host {
			return
		}
		d.entries[key] = host
		d.rebuildHosts()
		d.logger.Infof("[Discovery] %s add host: %s", d.service, host)

	case clientv3.EventTypeDelete:
		host, ok := d.entries[key]
		if !ok {
			return
		}
		delete(d.entries, key)
		d.rebuildHosts()
		d.logger.Infof("[Discovery] %s del host: %s", d.service, host)
	}
}

func (d *Discovery) rebuildHosts() {
	if len(d.entries) == 0 {
		empty := make([]string, 0)
		d.hosts.Store(&empty)
		return
	}

	set := make(map[string]struct{}, len(d.entries))
	hosts := make([]string, 0, len(d.entries))
	for _, host := range d.entries {
		if _, ok := set[host]; !ok {
			set[host] = struct{}{}
			hosts = append(hosts, host)
		}
	}
	slices.Sort(hosts)
	d.hosts.Store(&hosts)
}
