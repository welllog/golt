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

// AtomicLoad atomically loads a pointer field that is updated concurrently,
// e.g. a watch field of InitAndPreload or a value stored by AtomicStore.
// field is the address of the pointer field, like &c.field. Plain reads of such
// fields are data races. The result may be nil if the field is not loaded yet.
func AtomicLoad[T any](field **T) *T {
	return (*T)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(field))))
}
