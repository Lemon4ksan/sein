// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"encoding"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/lemon4ksan/foundation/silicon/hex"
	"github.com/lemon4ksan/foundation/types/uuid"
)

// TypedSetter writes a parsed string directly into field memory pointer with zero runtime reflection switch.
type TypedSetter func(ptr unsafe.Pointer, raw string) error

var (
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
	timeType            = reflect.TypeFor[time.Time]()
	durationType        = reflect.TypeFor[time.Duration]()
	netIPTypePtr        = reflect.TypeFor[net.IP]()
	netipAddrType       = reflect.TypeFor[netip.Addr]()
	uuidType            = reflect.TypeFor[uuid.UUID]()
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

// CompileSetter precompiles a typed, non-branching memory writer for typ and kind at startup.
func CompileSetter(
	typ reflect.Type,
	kind reflect.Kind,
	src ParamSource,
	key, format string,
	isHex, isBase64 bool,
) TypedSetter {
	if s := compileBinarySetter(typ, isHex, isBase64, src, key); s != nil {
		return s
	}

	if s := compileUnmarshalerSetter(typ, src, key); s != nil {
		return s
	}

	if s := compileSpecialTypeSetter(typ, src, key, format); s != nil {
		return s
	}

	if s := compileNumericSetter(kind, src, key); s != nil {
		return s
	}

	if s := compilePrimitiveSetter(kind, src, key); s != nil {
		return s
	}

	return func(ptr unsafe.Pointer, raw string) error {
		return nil
	}
}

func compileBinarySetter(typ reflect.Type, isHex, isBase64 bool, src ParamSource, key string) TypedSetter {
	if typ != bytesSliceType {
		return nil
	}

	if isHex {
		return func(ptr unsafe.Pointer, raw string) error {
			data, err := hex.DecodeString(raw)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*[]byte)(ptr) = data

			return nil
		}
	}

	if isBase64 {
		return func(ptr unsafe.Pointer, raw string) error {
			data, err := base64.StdEncoding.DecodeString(raw)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*[]byte)(ptr) = data

			return nil
		}
	}

	return nil
}

func compileUnmarshalerSetter(typ reflect.Type, src ParamSource, key string) TypedSetter {
	if typ.Implements(textUnmarshalerType) || reflect.PointerTo(typ).Implements(textUnmarshalerType) {
		return func(ptr unsafe.Pointer, raw string) error {
			val := reflect.NewAt(typ, ptr).Interface().(encoding.TextUnmarshaler)
			if err := val.UnmarshalText([]byte(raw)); err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			return nil
		}
	}

	return nil
}

func compileSpecialTypeSetter(typ reflect.Type, src ParamSource, key, format string) TypedSetter {
	switch typ {
	case uuidType:
		return func(ptr unsafe.Pointer, raw string) error {
			u, err := uuid.Parse(raw)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*uuid.UUID)(ptr) = u

			return nil
		}

	case timeType:
		return func(ptr unsafe.Pointer, raw string) error {
			t, err := ParseTime(raw, format)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*time.Time)(ptr) = t

			return nil
		}

	case durationType:
		return func(ptr unsafe.Pointer, raw string) error {
			d, err := time.ParseDuration(raw)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*time.Duration)(ptr) = d

			return nil
		}

	case netIPTypePtr:
		return func(ptr unsafe.Pointer, raw string) error {
			ip := net.ParseIP(raw)
			if ip == nil {
				return ScalarError{Source: src, Key: key, Cause: errors.New("invalid IP address")}
			}

			*(*net.IP)(ptr) = ip

			return nil
		}

	case netipAddrType:
		return func(ptr unsafe.Pointer, raw string) error {
			addr, err := netip.ParseAddr(raw)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*netip.Addr)(ptr) = addr

			return nil
		}

	default:
		return nil
	}
}

func compileNumericSetter(kind reflect.Kind, src ParamSource, key string) TypedSetter {
	switch kind {
	case reflect.Uint64, reflect.Uint:
		return func(ptr unsafe.Pointer, raw string) error {
			v, err := strconv.ParseUint(raw, 10, 64)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*uint64)(ptr) = v

			return nil
		}

	case reflect.Uint32:
		return func(ptr unsafe.Pointer, raw string) error {
			v, err := strconv.ParseUint(raw, 10, 32)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*uint32)(ptr) = uint32(v)

			return nil
		}

	case reflect.Uint16:
		return func(ptr unsafe.Pointer, raw string) error {
			v, err := strconv.ParseUint(raw, 10, 16)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*uint16)(ptr) = uint16(v)

			return nil
		}

	case reflect.Uint8:
		return func(ptr unsafe.Pointer, raw string) error {
			v, err := strconv.ParseUint(raw, 10, 8)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*uint8)(ptr) = uint8(v)

			return nil
		}

	case reflect.Int64, reflect.Int:
		return func(ptr unsafe.Pointer, raw string) error {
			v, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*int64)(ptr) = v

			return nil
		}

	case reflect.Int32:
		return func(ptr unsafe.Pointer, raw string) error {
			v, err := strconv.ParseInt(raw, 10, 32)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*int32)(ptr) = int32(v)

			return nil
		}

	case reflect.Int16:
		return func(ptr unsafe.Pointer, raw string) error {
			v, err := strconv.ParseInt(raw, 10, 16)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*int16)(ptr) = int16(v)

			return nil
		}

	case reflect.Int8:
		return func(ptr unsafe.Pointer, raw string) error {
			v, err := strconv.ParseInt(raw, 10, 8)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*int8)(ptr) = int8(v)

			return nil
		}

	case reflect.Float64:
		return func(ptr unsafe.Pointer, raw string) error {
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*float64)(ptr) = v

			return nil
		}

	case reflect.Float32:
		return func(ptr unsafe.Pointer, raw string) error {
			v, err := strconv.ParseFloat(raw, 32)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*float32)(ptr) = float32(v)

			return nil
		}

	default:
		return nil
	}
}

func compilePrimitiveSetter(kind reflect.Kind, src ParamSource, key string) TypedSetter {
	switch kind {
	case reflect.String:
		return func(ptr unsafe.Pointer, raw string) error {
			*(*string)(ptr) = raw
			return nil
		}

	case reflect.Bool:
		return func(ptr unsafe.Pointer, raw string) error {
			v, err := parseCustomBool(raw)
			if err != nil {
				return ScalarError{Source: src, Key: key, Cause: err}
			}

			*(*bool)(ptr) = v

			return nil
		}

	default:
		return nil
	}
}

// ParseTime parses a timestamp string using optional custom layout, RFC3339, DateOnly, or Unix millis.
func ParseTime(s, customFormat string) (time.Time, error) {
	if customFormat != "" {
		return time.Parse(customFormat, s)
	}

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
