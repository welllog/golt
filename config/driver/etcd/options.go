package etcd

import clientv3 "go.etcd.io/etcd/client/v3"

type Option func(*etcdDriverOption)

type etcdDriverOption struct {
	etcdClient         *clientv3.Client
	etcdConfig         clientv3.Config
	commonPrefixMinLen int
	preload            bool
	// when custom etcd client is provided, closeCustomEtcdClient indicates whether to close it when etcd driver is closed
	closeCustomEtcdClient bool
}

func WithEtcdConfig(config clientv3.Config) Option {
	return func(o *etcdDriverOption) {
		o.etcdConfig = config
	}
}

func WithCommonPrefixMinLen(len int) Option {
	return func(o *etcdDriverOption) {
		o.commonPrefixMinLen = len
	}
}

func WithPreload() Option {
	return func(o *etcdDriverOption) {
		o.preload = true
	}
}

func WithCustomEtcdClient(client *clientv3.Client) Option {
	return func(o *etcdDriverOption) {
		o.etcdClient = client
	}
}

func WithCloseCustomEtcdClient() Option {
	return func(o *etcdDriverOption) {
		o.closeCustomEtcdClient = true
	}
}
