package config

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/welllog/golt/config/driver"
	_ "github.com/welllog/golt/config/driver/etcd"
	_ "github.com/welllog/golt/config/driver/file"
	"github.com/welllog/golt/config/meta"
	"github.com/welllog/golt/contract"
)

var ErrNotFound = driver.ErrNotFound

type Configure struct {
	ds     map[string]driver.Driver
	logger contract.Logger
	// closeEtcdCli is only set on a successful build, so failure paths never
	// close a caller-owned client
	closeEtcdCli func()
}

func newConfigure(cfs []meta.Config, logger contract.Logger, overrides map[string]driver.Factory) (*Configure, error) {
	// reject duplicate namespaces before any driver is created: a duplicate
	// inside one source would otherwise be swallowed by driver construction
	seen := make(map[string]struct{})
	for _, c := range cfs {
		for _, rule := range c.Configs {
			for _, np := range rule.Namespaces() {
				if _, ok := seen[np]; ok {
					return nil, errors.New("duplicate namespace: " + np + " (source " + c.Source + ")")
				}
				seen[np] = struct{}{}
			}
		}
	}

	cfg := Configure{
		ds:     make(map[string]driver.Driver, len(cfs)*2),
		logger: logger,
	}

	for _, c := range cfs {
		d, err := driver.New(c, logger, overrides)
		if err != nil {
			logger.Errorf("new driver failed: %s %s", c.SourceSchema(), c.SourceAddr())
			cfg.Close()
			return nil, err
		}

		for _, v := range d.Namespaces() {
			_, ok := cfg.ds[v]
			if ok {
				d.Close()
				cfg.Close()
				logger.Errorf("duplicate namespace: %s on %s %s", v, c.SourceSchema(), c.SourceAddr())
				return nil, errors.New("duplicate namespace: " + v)
			}
			cfg.ds[v] = d
		}
	}

	return &cfg, nil
}

// OnKeyChange registers a hook to run when the key's value changes.
// It returns an error wrapping ErrNotFound when the namespace is unknown,
// or driver.ErrNotWatchable when watch is not enabled for the namespace.
func (c *Configure) OnKeyChange(namespace, key string, hook func([]byte) error) error {
	d, ok := c.ds[namespace]
	if !ok {
		return fmt.Errorf("unknown namespace %q: %w", namespace, ErrNotFound)
	}

	return d.OnKeyChange(namespace, key, hook)
}

// Namespaces returns all loaded namespaces, sorted.
func (c *Configure) Namespaces() []string {
	nps := make([]string, 0, len(c.ds))
	for np := range c.ds {
		nps = append(nps, np)
	}
	slices.Sort(nps)
	return nps
}

func (c *Configure) GetRaw(ctx context.Context, namespace, key string) ([]byte, error) {
	b, err := c.UnsafeGetRaw(ctx, namespace, key)
	if err != nil {
		return nil, err
	}

	return append([]byte(nil), b...), nil
}

func (c *Configure) UnsafeGetRaw(ctx context.Context, namespace, key string) ([]byte, error) {
	d, ok := c.ds[namespace]
	if !ok {
		return nil, ErrNotFound
	}

	return d.Get(ctx, namespace, key)
}

func (c *Configure) GetRawString(ctx context.Context, namespace, key string) (string, error) {
	d, ok := c.ds[namespace]
	if !ok {
		return "", ErrNotFound
	}

	return d.GetString(ctx, namespace, key)
}

func (c *Configure) String(ctx context.Context, namespace, key string) (string, error) {
	s, err := c.GetRawString(ctx, namespace, key)
	if err != nil {
		return "", err
	}

	return unquote(s), nil
}

func (c *Configure) Int64(ctx context.Context, namespace, key string) (int64, error) {
	s, err := c.String(ctx, namespace, key)
	if err != nil {
		return 0, err
	}

	return strconv.ParseInt(s, 10, 64)
}

func (c *Configure) Int(ctx context.Context, namespace, key string) (int, error) {
	s, err := c.String(ctx, namespace, key)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(s)
}

func (c *Configure) Float64(ctx context.Context, namespace, key string) (float64, error) {
	s, err := c.String(ctx, namespace, key)
	if err != nil {
		return 0, err
	}

	return strconv.ParseFloat(s, 64)
}

func (c *Configure) Bool(ctx context.Context, namespace, key string) (bool, error) {
	s, err := c.String(ctx, namespace, key)
	if err != nil {
		return false, err
	}

	return strconv.ParseBool(s)
}

func (c *Configure) StringOr(ctx context.Context, namespace, key, def string) string {
	s, err := c.String(ctx, namespace, key)
	if err != nil {
		return def
	}
	return s
}

func (c *Configure) IntOr(ctx context.Context, namespace, key string, def int) int {
	v, err := c.Int(ctx, namespace, key)
	if err != nil {
		return def
	}
	return v
}

func (c *Configure) Int64Or(ctx context.Context, namespace, key string, def int64) int64 {
	v, err := c.Int64(ctx, namespace, key)
	if err != nil {
		return def
	}
	return v
}

func (c *Configure) Float64Or(ctx context.Context, namespace, key string, def float64) float64 {
	v, err := c.Float64(ctx, namespace, key)
	if err != nil {
		return def
	}
	return v
}

func (c *Configure) BoolOr(ctx context.Context, namespace, key string, def bool) bool {
	v, err := c.Bool(ctx, namespace, key)
	if err != nil {
		return def
	}
	return v
}

func (c *Configure) YamlDecode(ctx context.Context, namespace, key string, value any) error {
	return c.Decode(ctx, namespace, key, value, driver.MustGetDecoder("yaml"))
}

func (c *Configure) JsonDecode(ctx context.Context, namespace, key string, value any) error {
	return c.Decode(ctx, namespace, key, value, driver.MustGetDecoder("json"))
}

func (c *Configure) TomlDecode(ctx context.Context, namespace, key string, value any) error {
	return c.Decode(ctx, namespace, key, value, driver.MustGetDecoder("toml"))
}

func (c *Configure) Decode(ctx context.Context, namespace, key string, value any, fn driver.Decoder) error {
	d, ok := c.ds[namespace]
	if !ok {
		return ErrNotFound
	}

	b, err := d.Get(ctx, namespace, key)
	if err != nil {
		return err
	}

	return fn(b, value)
}

// Close closes all drivers, then the custom etcd client if
// WithCloseCustomEtcdClient was set. It must not be called concurrently;
// repeated sequential calls are no-ops.
func (c *Configure) Close() {
	for _, v := range c.ds {
		v.Close()
	}

	if c.closeEtcdCli != nil {
		c.closeEtcdCli()
		c.closeEtcdCli = nil
	}
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	n := len(s)
	if n < 2 {
		return s
	}

	if s[0] == '"' && s[n-1] == '"' {
		if unquoted, err := strconv.Unquote(s); err == nil {
			return unquoted
		}
		return s[1 : n-1]
	}

	if s[0] == '\'' && s[n-1] == '\'' {
		return s[1 : n-1]
	}

	return s
}

// ScopedConfigure represents a view of Configure scoped to a specific namespace.
// It simplifies querying multiple keys under the same namespace without repeating
// the namespace string.
type ScopedConfigure struct {
	c         *Configure
	namespace string
}

// Namespace returns a ScopedConfigure bound to the given namespace.
func (c *Configure) Namespace(namespace string) ScopedConfigure {
	return ScopedConfigure{c: c, namespace: namespace}
}

// Name returns the bound namespace name.
func (s ScopedConfigure) Name() string {
	return s.namespace
}

func (s ScopedConfigure) GetRaw(ctx context.Context, key string) ([]byte, error) {
	return s.c.GetRaw(ctx, s.namespace, key)
}

func (s ScopedConfigure) UnsafeGetRaw(ctx context.Context, key string) ([]byte, error) {
	return s.c.UnsafeGetRaw(ctx, s.namespace, key)
}

func (s ScopedConfigure) GetRawString(ctx context.Context, key string) (string, error) {
	return s.c.GetRawString(ctx, s.namespace, key)
}

func (s ScopedConfigure) String(ctx context.Context, key string) (string, error) {
	return s.c.String(ctx, s.namespace, key)
}

func (s ScopedConfigure) Int(ctx context.Context, key string) (int, error) {
	return s.c.Int(ctx, s.namespace, key)
}

func (s ScopedConfigure) Int64(ctx context.Context, key string) (int64, error) {
	return s.c.Int64(ctx, s.namespace, key)
}

func (s ScopedConfigure) Float64(ctx context.Context, key string) (float64, error) {
	return s.c.Float64(ctx, s.namespace, key)
}

func (s ScopedConfigure) Bool(ctx context.Context, key string) (bool, error) {
	return s.c.Bool(ctx, s.namespace, key)
}

func (s ScopedConfigure) StringOr(ctx context.Context, key, def string) string {
	return s.c.StringOr(ctx, s.namespace, key, def)
}

func (s ScopedConfigure) IntOr(ctx context.Context, key string, def int) int {
	return s.c.IntOr(ctx, s.namespace, key, def)
}

func (s ScopedConfigure) Int64Or(ctx context.Context, key string, def int64) int64 {
	return s.c.Int64Or(ctx, s.namespace, key, def)
}

func (s ScopedConfigure) Float64Or(ctx context.Context, key string, def float64) float64 {
	return s.c.Float64Or(ctx, s.namespace, key, def)
}

func (s ScopedConfigure) BoolOr(ctx context.Context, key string, def bool) bool {
	return s.c.BoolOr(ctx, s.namespace, key, def)
}

func (s ScopedConfigure) YamlDecode(ctx context.Context, key string, value any) error {
	return s.c.YamlDecode(ctx, s.namespace, key, value)
}

func (s ScopedConfigure) JsonDecode(ctx context.Context, key string, value any) error {
	return s.c.JsonDecode(ctx, s.namespace, key, value)
}

func (s ScopedConfigure) TomlDecode(ctx context.Context, key string, value any) error {
	return s.c.TomlDecode(ctx, s.namespace, key, value)
}

func (s ScopedConfigure) Decode(ctx context.Context, key string, value any, fn driver.Decoder) error {
	return s.c.Decode(ctx, s.namespace, key, value, fn)
}

func (s ScopedConfigure) OnKeyChange(key string, hook func([]byte) error) error {
	return s.c.OnKeyChange(s.namespace, key, hook)
}

