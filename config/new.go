package config

import (
	"context"
	"errors"
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

func NewConfigure(cfs []meta.Config, options ...Option) (*Configure, error) {
	opts := configOptions{
		logger:  nil,
		etcdCli: nil,
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

	if opts.etcdCli != nil {
		driver.RegisterDriver("custom_etcd", func(c meta.Config, l contract.Logger) (driver.Driver, error) {
			return etcd.NewAdvanced(c, l, etcd.WithExistsEtcdClient(opts.etcdCli))
		})
	}

	return newConfigure(cfs, opts.logger)
}

func FromFile(file string, options ...Option) (*Configure, error) {
	var cs []meta.Config

	ext := strings.TrimPrefix(filepath.Ext(file), ".")
	fn, ok := formatterMap[ext]
	if !ok {
		return nil, errors.New("unsupported config file format")
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

func FromEtcd(cli *clientv3.Client, metaConfigKey string, fn UnmarshalFunc, options ...Option) (*Configure, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rsp, err := cli.Get(ctx, metaConfigKey)
	if err != nil {
		return nil, fmt.Errorf("get meta config from etcd failed: %w", err)
	}

	if len(rsp.Kvs) == 0 {
		return nil, errors.New("no meta config found in etcd")
	}

	var cs []meta.Config
	if err := fn(rsp.Kvs[0].Value, &cs); err != nil {
		return nil, fmt.Errorf("unmarshal meta config failed: %w", err)
	}

	return NewConfigure(cs, options...)
}
