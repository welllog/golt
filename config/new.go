package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/welllog/golt/config/driver"
	"github.com/welllog/golt/config/driver/etcd"
	"github.com/welllog/golt/config/meta"
	"github.com/welllog/golt/contract"
	"github.com/welllog/olog"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// NewConfigure creates a Configure from meta configs.
// When WithCustomEtcdClient is set, every etcd:// source in cfs reuses that
// client instead of dialing its own.
func NewConfigure(cfs []meta.Config, options ...Option) (*Configure, error) {
	opts := configOptions{
		logger:                      nil,
		etcdCli:                     nil,
		etcdWatchCommonPrefixMinLen: 0,
		etcdPreload:                 false,
		closeEtcdCli:                false,
	}
	for _, opt := range options {
		opt(&opts)
	}

	if opts.logger == nil {
		logger := olog.DynamicLogger{
			Caller: olog.Disable,
		}
		logger.SetAppName("golt.config")
		opts.logger = logger
	}

	var overrides map[string]driver.Factory
	if opts.etcdCli != nil || opts.etcdWatchCommonPrefixMinLen != 0 || opts.etcdPreload {
		etcdOpts := make([]etcd.Option, 0, 3)
		if opts.etcdWatchCommonPrefixMinLen != 0 {
			etcdOpts = append(etcdOpts, etcd.WithCommonPrefixMinLen(opts.etcdWatchCommonPrefixMinLen))
		}
		if opts.etcdPreload {
			etcdOpts = append(etcdOpts, etcd.WithPreload())
		}
		if opts.etcdCli != nil {
			etcdOpts = append(etcdOpts, etcd.WithCustomEtcdClient(opts.etcdCli))
		}

		overrides = map[string]driver.Factory{
			"etcd": func(c meta.Config, l contract.Logger) (driver.Driver, error) {
				return etcd.NewAdvanced(c, l, etcdOpts...)
			},
		}
	}

	cfg, err := newConfigure(cfs, opts.logger, overrides)
	if err != nil {
		return nil, err
	}

	// every etcd:// driver shares this client, so none of them may close it;
	// Close does it exactly once, and only after a successful build
	if opts.etcdCli != nil && opts.closeEtcdCli {
		cfg.closeEtcdCli = func() { _ = opts.etcdCli.Close() }
	}
	return cfg, nil
}

func FromFile(file string, options ...Option) (*Configure, error) {
	var cs []meta.Config

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(file), "."))
	fn, ok := driver.GetDecoder(ext)
	if !ok {
		return nil, fmt.Errorf("unsupported config file format: %s, "+
			"you can register custom format decoder by driver.RegisterDecoder", ext)
	}

	b, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", file, err)
	}

	if err := fn(b, &cs); err != nil {
		return nil, err
	}

	return NewConfigure(cs, options...)
}

func FromEtcdConfig(clientConfig clientv3.Config, metaConfigKey string, fn driver.Decoder, options ...Option) (*Configure, error) {
	cli, err := clientv3.New(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("create etcd client failed: %w", err)
	}

	options = append(options, WithCustomEtcdClient(cli), WithCloseCustomEtcdClient())
	cfg, err := FromEtcd(cli, metaConfigKey, fn, options...)
	if err != nil {
		cli.Close()
		return nil, err
	}
	return cfg, nil
}

func FromEtcd(cli *clientv3.Client, metaConfigKey string, fn driver.Decoder, options ...Option) (*Configure, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rsp, err := cli.Get(ctx, metaConfigKey)
	if err != nil {
		return nil, fmt.Errorf("get meta config from etcd failed: %w, meta config key: %s", err, metaConfigKey)
	}

	if len(rsp.Kvs) == 0 {
		return nil, fmt.Errorf("no meta config found in etcd key: %s", metaConfigKey)
	}

	var cs []meta.Config
	if err := fn(rsp.Kvs[0].Value, &cs); err != nil {
		return nil, fmt.Errorf("unmarshal meta config failed: %w, meta config key: %s", err, metaConfigKey)
	}

	return NewConfigure(cs, options...)
}
