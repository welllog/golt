package config

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/welllog/golt/config/driver"
)

type FieldLazyLoadMap map[unsafe.Pointer]func() error

// InitAndPreload parses the struct tags of the fields in the struct pointed to by dst,
// preloads the configuration values, and returns a map of lazy load functions for fields that are marked as lazy.
// The dst parameter must be a pointer to a struct.
// The struct fields can be either exported or unexported.
// For exported fields, the configuration value is directly set to the field.
// For unexported fields, if the field is not a pointer, the configuration value is directly set to the field using unsafe.
// For unexported pointer fields, if the lazy option is not set, the configuration value is loaded and set to the field using unsafe.
// If the lazy option is set for an unexported pointer field, a lazy load function is returned in the map.
// If the watch option is set for an unexported pointer field, a callback function is registered to update the field when the configuration changes.
func (c *Configure) InitAndPreload(dst any, fieldLoadTimeout time.Duration) (FieldLazyLoadMap, error) {
	begin := time.Now()

	v, err := validateDst(dst)
	if err != nil {
		return nil, err
	}

	funcs := FieldLazyLoadMap{}

	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		tag := field.Tag.Get("config")
		if tag == "" {
			continue
		}

		ct, err := parseConfigTag(tag)
		if err != nil {
			return nil, fmt.Errorf("field %s tag parse: %w", field.Name, err)
		}

		if field.IsExported() {
			err = c.loadExportedField(field, v.Field(i), ct, fieldLoadTimeout)
			if err != nil {
				return nil, err
			}
			continue
		}

		if field.Type.Kind() != reflect.Ptr {
			err = c.loadUnexportedNotPtrField(field, v.Field(i), ct, fieldLoadTimeout)
			if err != nil {
				return nil, err
			}
			continue
		}

		err = c.handleLazyOrWatchedField(field, v.Field(i), ct, funcs, fieldLoadTimeout)
		if err != nil {
			return nil, err
		}
	}

	c.logger.Debugf("InitAndPreload took %d ms", time.Since(begin).Milliseconds())
	return funcs, nil
}

// TryLoad attempts to load the configuration value for the field pointed to by fieldPtr.
// The fieldPtr must be a pointer to an unsafe.Pointer that points to the actual field, and the field must be of pointer type.
// fieldPtr like this: type User struct { addr *Addr `config:"..."` } -> unsafe.Pointer(&user.addr)
//
// If the field is already loaded (i.e., not nil), it returns the current value.
// If the field is not loaded and a corresponding load function exists in funcs, it invokes the load function to load the value.
// After invoking the load function, it checks again if the field is loaded and returns the value if successful.
// If no load function exists or loading fails, it returns an error.
func (c *Configure) TryLoad(fieldPtr unsafe.Pointer, funcs FieldLazyLoadMap) (unsafe.Pointer, error) {
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

func (c *Configure) loadExportedField(field reflect.StructField, fieldValue reflect.Value, ct configTag, loadTimeout time.Duration) error {
	if ct.Lazy || ct.Watch {
		c.logger.Warnf("Field %s is exported, lazy/watch options are ignored", field.Name)
	}

	loadCtx, loadCancel := context.WithTimeout(context.Background(), loadTimeout)
	if field.Type.Kind() == reflect.Ptr {
		ptrValue := reflect.New(field.Type.Elem())
		err := c.Decode(loadCtx, ct.Namespace, ct.Key, ptrValue.Interface(), formatter{f: ct.Format}.Decode)
		loadCancel()
		if err != nil {
			return fmt.Errorf("field %s preload failed: %w", field.Name, err)
		}
		fieldValue.Set(ptrValue)
		return nil
	}

	err := c.Decode(loadCtx, ct.Namespace, ct.Key, fieldValue.Addr().Interface(), formatter{f: ct.Format}.Decode)
	loadCancel()
	if err != nil {
		return fmt.Errorf("field %s preload failed: %w", field.Name, err)
	}
	return nil
}

func (c *Configure) loadUnexportedNotPtrField(field reflect.StructField, fieldValue reflect.Value, ct configTag, loadTimeout time.Duration) error {
	if ct.Lazy || ct.Watch {
		c.logger.Warnf("Field %s is not a pointer, lazy/watch options are ignored", field.Name)
	}

	dst := reflect.NewAt(field.Type, unsafe.Pointer(fieldValue.UnsafeAddr())).Interface()
	loadCtx, loadCancel := context.WithTimeout(context.Background(), loadTimeout)
	err := c.Decode(loadCtx, ct.Namespace, ct.Key, dst, formatter{f: ct.Format}.Decode)
	loadCancel()
	if err != nil {
		return fmt.Errorf("field %s preload failed: %w", field.Name, err)
	}
	return nil
}

func (c *Configure) handleLazyOrWatchedField(field reflect.StructField, fieldValue reflect.Value, ct configTag, funcs FieldLazyLoadMap, loadTimeout time.Duration) error {
	fieldPtr := fieldValue.Addr().UnsafePointer()
	fieldType := field.Type.Elem()
	if ct.Watch {
		callbackOk := c.OnKeyChange(ct.Namespace, ct.Key, func(b []byte) error {
			ptrValue := reflect.New(fieldType)
			err := formatter{f: ct.Format}.Decode(b, ptrValue.Interface())
			if err != nil {
				return err
			}

			atomic.StorePointer((*unsafe.Pointer)(fieldPtr), ptrValue.UnsafePointer())
			return nil
		})

		if !callbackOk {
			return fmt.Errorf("key: %s %s not watchable but %s is watched", ct.Namespace, ct.Key, field.Name)
		}
	}

	loadFunc := func() error {
		loadCtx, loadCancel := context.WithTimeout(context.Background(), loadTimeout)
		ptrValue := reflect.New(fieldType)
		err := c.Decode(loadCtx, ct.Namespace, ct.Key, ptrValue.Interface(), formatter{f: ct.Format}.Decode)
		loadCancel()
		if err != nil {
			return err
		}

		atomic.StorePointer((*unsafe.Pointer)(fieldPtr), ptrValue.UnsafePointer())
		return nil
	}

	if ct.Lazy {
		funcs[fieldPtr] = loadFunc
		return nil
	}

	err := loadFunc()
	if err != nil {
		return fmt.Errorf("field %s preload failed: %w", field.Name, err)
	}
	return nil
}

func validateDst(dst any) (reflect.Value, error) {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Ptr {
		return reflect.Value{}, fmt.Errorf("dst must be a pointer to struct")
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("dst must be a pointer to struct")
	}

	return v, nil
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
			return ct, fmt.Errorf("invalid config tag: %s", tag)
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
		return ct, fmt.Errorf("config tag must have key and namespace, tag: %s", tag)
	}

	return ct, nil
}
