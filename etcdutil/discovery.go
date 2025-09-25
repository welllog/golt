package etcdutil

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/welllog/golib/strz"
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
	}

	hosts := make([]string, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		v := string(kv.Value)
		_, _, err = net.SplitHostPort(v)
		if err != nil {
			logger.Warnf("[NewDiscovery] %s invalid host: %s, err: %v", serviceName, v, err)
			continue
		}

		hosts = append(hosts, v)
	}
	d.hosts.Store(&hosts)

	return d, nil
}

// NewDiscoveryWithWatch creates a new Discovery instance and starts watching for changes.
func NewDiscoveryWithWatch(ctx context.Context, etcd *clientv3.Client, serviceName string, logger contract.Logger) (*Discovery, error) {
	d, err := NewDiscovery(etcd, serviceName, logger)
	if err != nil {
		return nil, err
	}

	ch := etcd.Watch(ctx, serviceName, clientv3.WithPrefix(), clientv3.WithPrevKV())
	go func(ch clientv3.WatchChan) {
		for rsp := range ch {
			for _, ev := range rsp.Events {
				d.Handle(ev)
			}
		}
	}(ch)

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
		idx := int(d.n.Add(1)-1) % len(*p)
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
	switch ev.Type {
	case clientv3.EventTypePut:
		host := string(ev.Kv.Value)
		if _, _, err := net.SplitHostPort(host); err != nil {
			d.logger.Warnf("[Handle] %s invalid host: %s, err: %v", d.service, host, err)
			return
		}
		d.add(host)
	case clientv3.EventTypeDelete:
		if ev.PrevKv == nil {
			d.logger.Warnf("[Handle] %s delete event with no prev kv", d.service)
			return
		}
		d.del(strz.UnsafeString(ev.PrevKv.Value))
	}
}

func (d *Discovery) add(host string) {
	p := d.hosts.Load()

	for _, a := range *p {
		if a == host {
			return
		}
	}

	newHosts := make([]string, len(*p)+1)
	copy(newHosts, *p)
	newHosts[len(*p)] = host
	d.hosts.Store(&newHosts)
	d.logger.Infof("[Discovery] %s add host: %s", d.service, host)
}

func (d *Discovery) del(host string) {
	p := d.hosts.Load()

	idx := -1
	for i, a := range *p {
		if a == host {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}

	if len(*p) == 1 {
		d.hosts.Store(&[]string{})
	} else {
		newHosts := make([]string, len(*p)-1)
		copy(newHosts, (*p)[:idx])
		copy(newHosts[idx:], (*p)[idx+1:])
		d.hosts.Store(&newHosts)
	}
	d.logger.Infof("[Discovery] %s del host: %s", d.service, host)
}
