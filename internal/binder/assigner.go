// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"encoding"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

var (
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
	timeType            = reflect.TypeFor[time.Time]()
	durationType        = reflect.TypeFor[time.Duration]()
	netIPTypePtr        = reflect.TypeFor[net.IP]()
	netipAddrType       = reflect.TypeFor[netip.Addr]()
)

// ScalarError represents a value parsing or conversion error on a field.
type ScalarError struct {
	Source ParamSource
	Key    string
	Cause  error
}

func (e ScalarError) Error() string {
	return fmt.Sprintf("invalid value for field %q: %v", e.Key, e.Cause)
}

func (e ScalarError) Unwrap() error {
	return e.Cause
}

// CompileAssigner builds the optimized type assignment function for a field.
func CompileAssigner(b *FieldBinding) AssignerFunc {
	if b.IsSlice {
		sliceElemKind := b.SliceElemKind
		sliceElemType := b.FieldType.Elem()
		fieldType := b.FieldType
		source := b.Source
		key := b.Key
		hasMin := b.HasMin
		minVal := b.MinVal
		hasMax := b.HasMax
		maxVal := b.MaxVal

		transforms := CompileTransforms(b)

		return func(req RequestView, fieldPtr unsafe.Pointer, initialVal string) error {
			var vals []string
			if source == SourceQuery {
				q := req.RawURLQuery()
				if sliceVals, ok := q[key]; ok && len(sliceVals) > 0 {
					for _, sv := range sliceVals {
						for _, item := range strings.Split(sv, ",") {
							if item = strings.TrimSpace(item); item != "" {
								vals = append(vals, item)
							}
						}
					}
				}
			}

			if len(vals) == 0 && initialVal != "" {
				for _, item := range strings.Split(initialVal, ",") {
					if item = strings.TrimSpace(item); item != "" {
						vals = append(vals, item)
					}
				}
			}

			if hasMin && float64(len(vals)) < minVal {
				return ValidationError{Message: fmt.Sprintf("%s slice length must be at least %v", key, minVal)}
			}
			if hasMax && float64(len(vals)) > maxVal {
				return ValidationError{Message: fmt.Sprintf("%s slice length must be at most %v", key, maxVal)}
			}

			sliceVal := reflect.MakeSlice(fieldType, len(vals), len(vals))
			for i, v := range vals {
				for _, t := range transforms {
					v = t(v)
				}
				elemPtr := sliceVal.Index(i).Addr().UnsafePointer()
				if err := AssignScalar(elemPtr, sliceElemKind, sliceElemType, v, source, key); err != nil {
					return err
				}
			}

			reflect.NewAt(fieldType, fieldPtr).Elem().Set(sliceVal)
			return nil
		}
	}

	if b.IsPtr {
		elemKind := b.ElemKind
		elemType := b.FieldType.Elem()
		source := b.Source
		key := b.Key

		return func(req RequestView, fieldPtr unsafe.Pointer, raw string) error {
			valPtr := reflect.New(elemType).UnsafePointer()
			*(*unsafe.Pointer)(fieldPtr) = valPtr
			return AssignScalar(valPtr, elemKind, elemType, raw, source, key)
		}
	}

	kind := b.Kind
	typ := b.FieldType
	source := b.Source
	key := b.Key

	return func(req RequestView, fieldPtr unsafe.Pointer, raw string) error {
		return AssignScalar(fieldPtr, kind, typ, raw, source, key)
	}
}

// AssignScalar parses and assigns a string representation to a typed scalar pointer.
func AssignScalar(ptr unsafe.Pointer, kind reflect.Kind, typ reflect.Type, s string, src ParamSource, key string) error {
	if typ.Implements(textUnmarshalerType) {
		val := reflect.NewAt(typ, ptr).Interface().(encoding.TextUnmarshaler)
		if err := val.UnmarshalText([]byte(s)); err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		return nil
	}
	if reflect.PointerTo(typ).Implements(textUnmarshalerType) {
		val := reflect.NewAt(typ, ptr).Interface().(encoding.TextUnmarshaler)
		if err := val.UnmarshalText([]byte(s)); err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		return nil
	}

	switch typ {
	case timeType:
		t, err := parseTime(s)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*time.Time)(ptr) = t
		return nil

	case durationType:
		d, err := time.ParseDuration(s)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*time.Duration)(ptr) = d
		return nil

	case netIPTypePtr:
		ip := net.ParseIP(s)
		if ip == nil {
			return ScalarError{Source: src, Key: key, Cause: errors.New("invalid IP address")}
		}
		*(*net.IP)(ptr) = ip
		return nil

	case netipAddrType:
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*netip.Addr)(ptr) = addr
		return nil
	}

	// 3. Built-in scalar kinds
	switch kind {
	case reflect.String:
		*(*string)(ptr) = s

	case reflect.Uint64, reflect.Uint:
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*uint64)(ptr) = v

	case reflect.Uint32:
		v, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*uint32)(ptr) = uint32(v)

	case reflect.Uint16:
		v, err := strconv.ParseUint(s, 10, 16)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*uint16)(ptr) = uint16(v)

	case reflect.Uint8:
		v, err := strconv.ParseUint(s, 10, 8)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*uint8)(ptr) = uint8(v)

	case reflect.Int64:
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*int64)(ptr) = v

	case reflect.Int:
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*int)(ptr) = int(v)

	case reflect.Int32:
		v, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*int32)(ptr) = int32(v)

	case reflect.Int16:
		v, err := strconv.ParseInt(s, 10, 16)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*int16)(ptr) = int16(v)

	case reflect.Int8:
		v, err := strconv.ParseInt(s, 10, 8)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*int8)(ptr) = int8(v)

	case reflect.Bool:
		v, err := parseCustomBool(s)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*bool)(ptr) = v

	case reflect.Float64:
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*float64)(ptr) = v

	case reflect.Float32:
		v, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return ScalarError{Source: src, Key: key, Cause: err}
		}
		*(*float32)(ptr) = float32(v)
	}
	return nil
}

func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t, nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1e11 {
			return time.UnixMilli(n), nil
		}
		return time.Unix(n, 0), nil
	}
	return time.Time{}, errors.New("invalid date/time format")
}

func parseCustomBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "yes", "y", "on":
		return true, nil
	case "0", "f", "false", "no", "n", "off", "":
		return false, nil
	default:
		return false, strconv.ErrSyntax
	}
}
