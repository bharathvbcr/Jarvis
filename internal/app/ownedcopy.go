package app

import (
	"reflect"
	"sync"
)

// ownedCopy returns a copy of v that shares nothing a caller could write
// through: every pointer target, slice backing array and map in it is new.
// Strings are shared, because they are immutable; that is what makes copying a
// run view cheap even when it holds a multi-megabyte frame.
//
// It copies exported fields only. A struct with an unexported field that holds
// a pointer, slice or map would stay shared, so the types handed to ownedCopy
// must not have one; TestOwnedCopyTypesHaveNoHiddenReferences enforces that for
// RunView and computer.Record.
func ownedCopy[T any](v T) T {
	detach(reflect.ValueOf(&v).Elem())
	return v
}

// detach replaces every reference reachable from v with a private copy. v must
// be settable.
func detach(v reflect.Value) {
	if !holdsReference(v.Type()) {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		n := reflect.New(v.Type().Elem())
		n.Elem().Set(v.Elem())
		detach(n.Elem())
		v.Set(n)
	case reflect.Slice:
		if v.IsNil() {
			return
		}
		n := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		reflect.Copy(n, v)
		if holdsReference(v.Type().Elem()) {
			for i := 0; i < n.Len(); i++ {
				detach(n.Index(i))
			}
		}
		v.Set(n)
	case reflect.Map:
		if v.IsNil() {
			return
		}
		n := reflect.MakeMapWithSize(v.Type(), v.Len())
		for iter := v.MapRange(); iter.Next(); {
			e := reflect.New(v.Type().Elem()).Elem()
			e.Set(iter.Value())
			detach(e)
			n.SetMapIndex(iter.Key(), e)
		}
		v.Set(n)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			detach(v.Index(i))
		}
	case reflect.Interface:
		if v.IsNil() {
			return
		}
		n := reflect.New(v.Elem().Type()).Elem()
		n.Set(v.Elem())
		detach(n)
		v.Set(n)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if f := v.Field(i); f.CanSet() {
				detach(f)
			}
		}
	}
}

var referenceTypes sync.Map // reflect.Type -> bool

// holdsReference reports whether a value of type t can alias memory another
// value also reaches: it is, or contains, a pointer, slice, map or interface.
func holdsReference(t reflect.Type) bool {
	if cached, ok := referenceTypes.Load(t); ok {
		return cached.(bool)
	}
	// Only pointers, slices, maps and interfaces can make a type recursive, and
	// they answer true without looking at their element, so this terminates.
	holds := false
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
		holds = true
	case reflect.Array:
		holds = holdsReference(t.Elem())
	case reflect.Struct:
		for i := 0; i < t.NumField() && !holds; i++ {
			holds = holdsReference(t.Field(i).Type)
		}
	}
	referenceTypes.Store(t, holds)
	return holds
}
