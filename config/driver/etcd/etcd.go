package etcd

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/welllog/golib/dsz"
	"github.com/welllog/golt/config/driver"
	"github.com/welllog/golt/config/meta"
	"github.com/welllog/golt/contract"
	"github.com/welllog/golt/etcdutil"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func init() {
	driver.RegisterDriver("etcd", New)
}

var _ driver.Driver = (*etcd)(nil)

type etcd struct {
	client         *clientv3.Client
	namespace2node map[string]*etcdutil.Kv
	watcher        *etcdutil.Watcher
}

func New(c meta.Config, logger contract.Logger) (driver.Driver, error) {
	return NewAdvanced(c, logger)
}

func NewAdvanced(c meta.Config, logger contract.Logger, options ...Option) (driver.Driver, error) {
	opts := etcdDriverOption{}
	for _, opt := range options {
		opt(&opts)
	}

	if opts.etcdClient == nil {
		if len(opts.etcdConfig.Endpoints) == 0 {
			opts.etcdConfig.Endpoints = strings.Split(c.SourceAddr(), ",")
		}

		if opts.etcdConfig.DialTimeout == 0 {
			opts.etcdConfig.DialTimeout = time.Minute
		}

		if opts.etcdConfig.DialKeepAliveTime == 0 {
			opts.etcdConfig.DialKeepAliveTime = 2 * time.Minute
		}

		if opts.etcdConfig.DialKeepAliveTimeout == 0 {
			opts.etcdConfig.DialKeepAliveTimeout = time.Minute
		}

		client, err := clientv3.New(opts.etcdConfig)
		if err != nil {
			return nil, err
		}
		opts.etcdClient = client
	}

	if opts.commonPrefixMinLen == 0 {
		opts.commonPrefixMinLen = 4
	}

	ed := etcd{
		client:         opts.etcdClient,
		namespace2node: make(map[string]*etcdutil.Kv, len(c.Configs)),
	}

	watcher := etcdutil.NewWatcher(opts.etcdClient).SetCommonPrefixMinLen(opts.commonPrefixMinLen).SetLogger(logger)
	path2node := make(map[string]*etcdutil.Kv, len(c.Configs))
	watchPath := make(dsz.Set[string], len(c.Configs))

	for _, cfg := range c.Configs {
		nps := cfg.Namespaces()

		node, ok := path2node[cfg.Path]
		if !ok {
			node = etcdutil.NewKv(cfg.Path, opts.etcdClient).SetLogger(logger)
			path2node[cfg.Path] = node

			if opts.preload || !cfg.Dynamic {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				err := node.Preload(ctx)
				cancel()
				if err != nil {
					_ = opts.etcdClient.Close()
					return nil, err
				}
			}
		}

		if cfg.Dynamic {
			if watchPath.Add(cfg.Path) {
				watcher.Attach(node)
			}
		}

		for _, np := range nps {
			ed.namespace2node[np] = node
		}
	}

	if len(ed.namespace2node) == 0 {
		return nil, errors.New("config rules is empty")
	}

	if len(watchPath) > 0 {
		ed.watcher = watcher
		watcher.Run(context.Background())
	}

	return &ed, nil
}

func (e *etcd) Namespaces() []string {
	nps := make([]string, 0, len(e.namespace2node))
	for np := range e.namespace2node {
		nps = append(nps, np)
	}
	return nps
}

func (e *etcd) OnKeyChange(namespace, key string, hook func([]byte) error) bool {
	node, ok := e.namespace2node[namespace]
	if !ok {
		return false
	}

	if !e.watcher.HasObserver(node.Prefix()) {
		return false
	}

	return node.OnKeyChange(key, hook)
}

func (e *etcd) Get(namespace, key string) ([]byte, error) {
	node, ok := e.namespace2node[namespace]
	if !ok {
		return nil, driver.ErrNotFound
	}

	b, err := node.UnsafeGet(context.Background(), key)
	if err != nil {
		if errors.Is(err, etcdutil.ErrNotFound) {
			return nil, driver.ErrNotFound
		}

		return nil, err
	}

	return b, nil
}

func (e *etcd) GetString(namespace, key string) (string, error) {
	node, ok := e.namespace2node[namespace]
	if !ok {
		return "", driver.ErrNotFound
	}

	value, err := node.GetString(context.Background(), key)
	if err != nil {
		if errors.Is(err, etcdutil.ErrNotFound) {
			return "", driver.ErrNotFound
		}

		return "", err
	}

	return value, nil
}

func (e *etcd) Close() {
	_ = e.client.Close()
}
