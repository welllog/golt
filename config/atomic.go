package config

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"unsafe"
)

func AtomicStore(ctx context.Context, cfg *Configure, namespace, key string, dst any, unmarshalFunc func([]byte, any) error) error {
	val := reflect.ValueOf(dst)
	if val.Kind() != reflect.Ptr {
		return errors.New("dst must be a pointer")
	}

	elem := val.Elem()
	if elem.Kind() != reflect.Ptr {
		return errors.New("dst must be a pointer to a pointer")
	}

	storeVal := reflect.New(elem.Type())
	err := cfg.Decode(ctx, namespace, key, storeVal.Interface(), unmarshalFunc)
	if err != nil {
		return err
	}

	atomic.StorePointer((*unsafe.Pointer)(val.UnsafePointer()), storeVal.Elem().UnsafePointer())
	return nil
}

func AtomicLoad(src any) unsafe.Pointer {
	val := reflect.ValueOf(src)
	return atomic.LoadPointer((*unsafe.Pointer)(val.UnsafePointer()))
}
