package config

import (
	"github.com/welllog/golt/contract"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Option func(*configOptions)

type configOptions struct {
	logger                      contract.Logger
	etcdCli                     *clientv3.Client
	etcdWatchCommonPrefixMinLen int
	etcdPreload                 bool
}

func WithLogger(logger contract.Logger) Option {
	return func(opts *configOptions) {
		opts.logger = logger
	}
}

func WithCustomEtcdClient(client *clientv3.Client) Option {
	return func(opts *configOptions) {
		opts.etcdCli = client
	}
}

func WithEtcdWatchCommonPrefixMinLen(length int) Option {
	return func(opts *configOptions) {
		opts.etcdWatchCommonPrefixMinLen = length
	}
}

func WithEtcdPreload() Option {
	return func(opts *configOptions) {
		opts.etcdPreload = true
	}
}
