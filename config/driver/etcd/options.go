package etcd

import clientv3 "go.etcd.io/etcd/client/v3"

type Option func(*etcdDriverOption)

type etcdDriverOption struct {
	etcdConfig         clientv3.Config
	commonPrefixMinLen int
	preload            bool
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
