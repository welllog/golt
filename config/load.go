package config

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/welllog/golt/config/driver"
	"gopkg.in/yaml.v3"
)

type FieldLoadFuncs map[unsafe.Pointer]func() error

func (c *Configure) InitAndPreload(dst any) error {
	v := reflect.ValueOf(dst)
	if v.Type().Kind() != reflect.Ptr {
		return fmt.Errorf("dst must be a pointer to struct")
	}

	v = v.Elem()
	t := v.Type()
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("dst must be a pointer to struct")
	}

	funcs := FieldLoadFuncs{}

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("config")
		if tag == "" {
			continue
		}

		ct, err := parseConfigTag(tag)
		if err != nil {
			return fmt.Errorf("field %s tag parse: %w", field.Name, err)
		}

		ft := field.Type
		ftKind := ft.Kind()
		isExported := field.IsExported()

		if isExported {
			if ct.Lazy {
				c.logger.Warnf("Field %s is exported, lazy option is ignored", field.Name)
				ct.Lazy = false
			}
			if ct.Watch {
				c.logger.Warnf("Field %s is exported, watch option is ignored", field.Name)
				ct.Watch = false
			}
		} else if ftKind != reflect.Ptr {
			if ct.Lazy {
				c.logger.Warnf("Field %s is not a pointer, lazy option is ignored", field.Name)
				ct.Lazy = false
			}
			if ct.Watch {
				c.logger.Warnf("Field %s is not a pointer, watch option is ignored", field.Name)
				ct.Watch = false
			}
		}

		if isExported {
			loadCtx, loadCancel := context.WithTimeout(context.Background(), 5*time.Second)
			if ftKind == reflect.Ptr {
				rft := ft.Elem()
				fv := reflect.New(rft)
				err = c.Decode(loadCtx, ct.Namespace, ct.Key, fv.Interface(), formatter{f: ct.Format}.Decode)
				loadCancel()
				if err != nil {
					return fmt.Errorf("field %s preload failed: %w", field.Name, err)
				}

				*(*unsafe.Pointer)(v.Field(i).Addr().UnsafePointer()) = unsafe.Pointer(fv.Elem().UnsafeAddr())

				continue
			}

			err = c.Decode(loadCtx, ct.Namespace, ct.Key, v.Field(i).Addr().Interface(), formatter{f: ct.Format}.Decode)
			loadCancel()
			if err != nil {
				return fmt.Errorf("field %s preload failed: %w", field.Name, err)
			}
			continue
		}

		if ftKind != reflect.Ptr {
			fdst := reflect.NewAt(ft, unsafe.Pointer(v.Field(i).UnsafeAddr())).Interface()
			loadCtx, loadCancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = c.Decode(loadCtx, ct.Namespace, ct.Key, fdst, formatter{f: ct.Format}.Decode)
			loadCancel()
			if err != nil {
				return fmt.Errorf("field %s preload failed: %w", field.Name, err)
			}
			continue
		}

		rft := ft.Elem()
		fieldPtr := v.Field(i).Addr().UnsafePointer()
		if ct.Watch {
			callbackOk := c.OnKeyChange(ct.Namespace, ct.Key, func(b []byte) error {
				fv := reflect.New(rft)
				err = formatter{f: ct.Format}.Decode(b, fv.Interface())
				if err != nil {
					return err
				}

				atomic.StorePointer((*unsafe.Pointer)(fieldPtr), unsafe.Pointer(fv.Elem().UnsafeAddr()))
				return nil
			})

			if !callbackOk {
				return fmt.Errorf("key: %s %s not watchable but %s is watched", ct.Namespace, ct.Key, field.Name)
			}
		}

		loadFunc := func() error {
			loadCtx, loadCancel := context.WithTimeout(context.Background(), 5*time.Second)
			fv := reflect.New(rft)
			err = c.Decode(loadCtx, ct.Namespace, ct.Key, fv.Interface(), formatter{f: ct.Format}.Decode)
			loadCancel()
			if err != nil {
				return err
			}

			atomic.StorePointer((*unsafe.Pointer)(fieldPtr), unsafe.Pointer(fv.Elem().UnsafeAddr()))
			return nil
		}

		if ct.Lazy {
			funcs[fieldPtr] = loadFunc
			continue
		}

		err = loadFunc()
		if err != nil {
			return fmt.Errorf("field %s preload failed: %w", field.Name, err)
		}
	}

	return nil
}

func (c *Configure) TryLoad(fieldPtr unsafe.Pointer, funcs FieldLoadFuncs) (unsafe.Pointer, error) {
	ptr := atomic.LoadPointer((*unsafe.Pointer)(fieldPtr))
	if ptr != nil {
		return ptr, nil
	}

	loadFunc, ok := funcs[fieldPtr]
	if !ok {
		return nil, driver.ErrNotFound
	}

	err := loadFunc()
	if err != nil {
		return nil, err
	}

	ptr = atomic.LoadPointer((*unsafe.Pointer)(fieldPtr))
	if ptr == nil {
		return nil, driver.ErrNotFound
	}
	return ptr, nil
}

type configTag struct {
	Namespace string
	Key       string
	Format    string
	Lazy      bool
	Watch     bool
}

func parseConfigTag(tag string) (configTag, error) {
	var ct configTag
	kvs := strings.Split(tag, ";")
	for _, kv := range kvs {
		if strings.TrimSpace(kv) == "" {
			continue
		}

		i := strings.Index(kv, ":")
		if i <= 0 {
			return ct, fmt.Errorf("invalid config tag")
		}
		key := strings.TrimSpace(kv[:i])
		value := strings.TrimSpace(kv[i+1:])
		switch key {
		case "key":
			ct.Key = value
		case "namespace":
			ct.Namespace = value
		case "format":
			ct.Format = value
		case "lazy":
			lazy, err := strconv.ParseBool(value)
			if err != nil {
				return ct, fmt.Errorf("invalid lazy value in config tag: %s", value)
			}
			ct.Lazy = lazy
		case "watch":
			watch, err := strconv.ParseBool(value)
			if err != nil {
				return ct, fmt.Errorf("invalid watch value in config tag: %s", value)
			}
			ct.Watch = watch
		}
	}

	if ct.Key == "" || ct.Namespace == "" {
		return ct, fmt.Errorf("config tag must have key and namespace")
	}

	return ct, nil
}

type formatter struct {
	f string
}

func (f formatter) Decode(b []byte, dst any) error {
	switch f.f {
	case "json":
		return json.Unmarshal(b, dst)
	default:
		return yaml.Unmarshal(b, dst)
	}
}
