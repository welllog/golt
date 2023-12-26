package etcdutil

import (
	"context"
	"errors"

	"github.com/welllog/golib/mapz"
	"github.com/welllog/golib/strz"
	"go.etcd.io/etcd/client/v3"
)

var ErrNotFound = errors.New("not found")

type Kv struct {
	prefix string
	kv     *mapz.SafeKV[string, []byte]
	client *clientv3.Client
}

func NewKv(prefix string, client *clientv3.Client) *Kv {
	kv := Kv{
		prefix: prefix,
		kv:     mapz.NewSafeKV[string, []byte](5),
		client: client,
	}
	return &kv
}

func (k *Kv) Prefix() string {
	return k.prefix
}

func (k *Kv) Preload(ctx context.Context) error {
	rsp, err := k.client.Get(ctx, k.prefix, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	l := len(k.prefix)
	k.kv.Map(func(kv mapz.KV[string, []byte]) {
		for _, v := range rsp.Kvs {
			kv.Set(string(v.Key[l:]), v.Value)
		}
	})
	return nil
}

func (k *Kv) Get(ctx context.Context, key string) ([]byte, error) {
	b, err := k.UnsafeGet(ctx, key)
	if err != nil {
		return nil, err
	}

	return append([]byte(nil), b...), nil
}

func (k *Kv) UnsafeGet(ctx context.Context, key string) ([]byte, error) {
	b, ok := k.kv.Get(key)
	if ok {
		if b == nil {
			return nil, ErrNotFound
		}

		return b, nil
	}

	b, err := k.GetNoCache(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			k.kv.Set(key, nil)
		}
		return nil, err
	}

	k.kv.Set(key, b)
	return b, nil
}

func (k *Kv) GetNoCache(ctx context.Context, key string) ([]byte, error) {
	rsp, err := k.client.Get(ctx, k.prefix+key)
	if err != nil {
		return nil, err
	}

	if len(rsp.Kvs) == 0 {
		return nil, ErrNotFound
	}
	return rsp.Kvs[0].Value, nil
}

func (k *Kv) Len() int {
	return k.kv.Len()
}

func (k *Kv) Handle(event *clientv3.Event) {
	switch event.Type {
	case clientv3.EventTypePut:
		k.kv.Set(string(event.Kv.Key[len(k.prefix):]), event.Kv.Value)
	case clientv3.EventTypeDelete:
		k.kv.Delete(strz.UnsafeString(event.Kv.Key[len(k.prefix):]))
	default:
	}
}
