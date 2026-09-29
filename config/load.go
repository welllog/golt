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
// fieldLoadTimeout bounds each field load; a non-positive value means no timeout.
// If the lazy option is set for an unexported pointer field, a lazy load function is returned in the map.
// If the watch option is set for an unexported pointer field, a callback function is registered to update the field when the configuration changes.
// Fields with watch:true are updated concurrently and MUST be read atomically,
// e.g. AtomicLoad(&c.field); plain reads are data races. Fields without
// watch can be read directly after InitAndPreload (or TryLoad for lazy fields) returns.
// Note: lazy and watch are only supported on unexported pointer fields; marking
// them on exported or non-pointer fields returns an error.
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

// TryLoad atomically loads a lazy field.
// field is the address of the pointer field, e.g. &c.addr.
// The result is typed and safe for direct access.
func TryLoad[T any](field **T, funcs FieldLazyLoadMap) (*T, error) {
	fieldPtr := unsafe.Pointer(field)
	ptr := atomic.LoadPointer((*unsafe.Pointer)(fieldPtr))
	if ptr != nil {
		return (*T)(ptr), nil
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
	return (*T)(ptr), nil
}

// loadContext returns a context for a field load. A non-positive timeout
// means no timeout, matching the zero-value convention of the rest of the lib
// (context.WithTimeout would expire immediately for d <= 0).
func loadContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(context.Background(), timeout)
	}
	return context.Background(), func() {}
}

func (c *Configure) loadExportedField(field reflect.StructField, fieldValue reflect.Value, ct configTag, loadTimeout time.Duration) error {
	if ct.Lazy || ct.Watch {
		return fmt.Errorf("field %s is exported: lazy and watch are only supported on unexported pointer fields", field.Name)
	}

	loadCtx, loadCancel := loadContext(loadTimeout)
	if field.Type.Kind() == reflect.Ptr {
		ptrValue := reflect.New(field.Type.Elem())
		err := c.Decode(loadCtx, ct.Namespace, ct.Key, ptrValue.Interface(), driver.GetDecoderOrDefault(ct.Format))
		loadCancel()
		if err != nil {
			return fmt.Errorf("field %s preload failed: %w", field.Name, err)
		}
		fieldValue.Set(ptrValue)
		return nil
	}

	err := c.Decode(loadCtx, ct.Namespace, ct.Key, fieldValue.Addr().Interface(), driver.GetDecoderOrDefault(ct.Format))
	loadCancel()
	if err != nil {
		return fmt.Errorf("field %s preload failed: %w", field.Name, err)
	}
	return nil
}

func (c *Configure) loadUnexportedNotPtrField(field reflect.StructField, fieldValue reflect.Value, ct configTag, loadTimeout time.Duration) error {
	if ct.Lazy || ct.Watch {
		return fmt.Errorf("field %s is not a pointer: lazy and watch are only supported on unexported pointer fields", field.Name)
	}

	dst := reflect.NewAt(field.Type, unsafe.Pointer(fieldValue.UnsafeAddr())).Interface()
	loadCtx, loadCancel := loadContext(loadTimeout)
	err := c.Decode(loadCtx, ct.Namespace, ct.Key, dst, driver.GetDecoderOrDefault(ct.Format))
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
		callbackErr := c.OnKeyChange(ct.Namespace, ct.Key, func(b []byte) error {
			ptrValue := reflect.New(fieldType)
			fn := driver.GetDecoderOrDefault(ct.Format)
			err := fn(b, ptrValue.Interface())
			if err != nil {
				return err
			}

			atomic.StorePointer((*unsafe.Pointer)(fieldPtr), ptrValue.UnsafePointer())
			return nil
		})
		if callbackErr != nil {
			return fmt.Errorf("field %s: %w", field.Name, callbackErr)
		}
	}

	loadFunc := func() error {
		loadCtx, loadCancel := loadContext(loadTimeout)
		ptrValue := reflect.New(fieldType)
		err := c.Decode(loadCtx, ct.Namespace, ct.Key, ptrValue.Interface(), driver.GetDecoderOrDefault(ct.Format))
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
