// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"unsafe"
)

// Common error sentinels returned by binder engine.
var (
	ErrMissingPathParam       = errors.New("required path parameter is missing")
	ErrMissingQueryParam      = errors.New("required query parameter is missing")
	ErrMissingHeader          = errors.New("required header is missing")
	ErrMissingCookie          = errors.New("required cookie is missing")
	ErrInvalidCookieSignature = errors.New("invalid or tampered cookie signature")
	ErrMissingBearerToken     = errors.New("authorization bearer token is required")
	ErrMissingContext         = errors.New("required context value is missing")
	ErrEmptyRequestBody       = errors.New("request body cannot be empty")
)

// Ingest executes the precompiled field steps across dest with zero runtime switch or allocations.
func Ingest[T any](req RequestView, dest *T) error {
	if dest == nil {
		return errors.New("dest must be a non-nil pointer")
	}

	if ing, ok := any(dest).(Ingestable); ok {
		if err := ing.IngestAny(req); err != nil {
			return err
		}

		return RunValidation(dest)
	}

	typ := reflect.TypeFor[T]()

	desc := GetDescriptor(typ)
	if desc == nil {
		bound, err := bindScalar(req, dest)
		if err != nil {
			return err
		}
		if bound {
			return RunValidation(dest)
		}

		if len(req.Body()) > 0 {
			if err := req.BindJSON(dest); err != nil {
				return err
			}
		}

		return RunValidation(dest)
	}

	ptr := unsafe.Pointer(dest)
	for i := range desc.Steps {
		if err := desc.Steps[i](req, ptr); err != nil {
			return err
		}
	}

	if desc.HasBodyFields && len(req.Body()) > 0 {
		if err := req.BindJSON(dest); err != nil {
			return err
		}
	}

	for i := range desc.PostValidators {
		if err := desc.PostValidators[i](ptr); err != nil {
			return err
		}
	}

	return RunValidation(dest)
}

// IngestType dynamically ingests request data into dest pointer for reflect.Type.
func IngestType(req RequestView, typ reflect.Type, dest any) error {
	if dest == nil {
		return errors.New("dest must be a non-nil pointer")
	}

	if ing, ok := dest.(Ingestable); ok {
		if err := ing.IngestAny(req); err != nil {
			return err
		}

		return RunValidation(dest)
	}

	desc := GetDescriptor(typ)
	if desc == nil {
		bound, err := bindScalarValue(req, typ, dest)
		if err != nil {
			return err
		}
		if bound {
			return RunValidation(dest)
		}

		if len(req.Body()) > 0 {
			if err := req.BindJSON(dest); err != nil {
				return err
			}
		}

		return RunValidation(dest)
	}

	ptr := unsafe.Pointer(reflect.ValueOf(dest).Pointer())
	for i := range desc.Steps {
		if err := desc.Steps[i](req, ptr); err != nil {
			return err
		}
	}

	if desc.HasBodyFields && len(req.Body()) > 0 {
		if err := req.BindJSON(dest); err != nil {
			return err
		}
	}

	for i := range desc.PostValidators {
		if err := desc.PostValidators[i](ptr); err != nil {
			return err
		}
	}

	return RunValidation(dest)
}

// IngestScalarType extracts the path parameter and parses it into scalar dest.
func IngestScalarType(req RequestView, typ reflect.Type, dest any) error {
	raw := req.Param("")
	if raw == "" {
		return ErrMissingPathParam
	}

	if unmarshaler, ok := dest.(encoding.TextUnmarshaler); ok {
		if err := unmarshaler.UnmarshalText([]byte(raw)); err != nil {
			return fmt.Errorf("invalid scalar format: %w", err)
		}

		return nil
	}

	elem := reflect.ValueOf(dest).Elem()
	k := typ.Kind()

	switch k {
	case reflect.String:
		elem.SetString(raw)
		return nil
	case reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8, reflect.Uint:
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid unsigned integer: %w", err)
		}
		elem.SetUint(v)
		return nil
	case reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8, reflect.Int:
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer: %w", err)
		}
		elem.SetInt(v)
		return nil
	case reflect.Bool:
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("invalid boolean: %w", err)
		}
		elem.SetBool(v)
		return nil
	case reflect.Float64, reflect.Float32:
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return fmt.Errorf("invalid float: %w", err)
		}
		elem.SetFloat(v)
		return nil
	}

	return fmt.Errorf("unsupported scalar type: %s", typ.String())
}

func bindScalar[T any](req RequestView, dest *T) (bool, error) {
	return bindScalarValue(req, reflect.TypeFor[T](), dest)
}

func bindScalarValue(req RequestView, typ reflect.Type, dest any) (bool, error) {
	raw := req.Param("")
	if raw == "" {
		return false, nil
	}

	if unmarshaler, ok := dest.(encoding.TextUnmarshaler); ok {
		if err := unmarshaler.UnmarshalText([]byte(raw)); err != nil {
			return true, fmt.Errorf("invalid scalar format: %w", err)
		}

		return true, nil
	}

	elem := reflect.ValueOf(dest).Elem()
	k := typ.Kind()

	switch k {
	case reflect.String:
		elem.SetString(raw)
		return true, nil
	case reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8, reflect.Uint:
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return true, fmt.Errorf("invalid unsigned integer: %w", err)
		}
		elem.SetUint(v)
		return true, nil
	case reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8, reflect.Int:
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return true, fmt.Errorf("invalid integer: %w", err)
		}
		elem.SetInt(v)
		return true, nil
	case reflect.Bool:
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return true, fmt.Errorf("invalid boolean: %w", err)
		}
		elem.SetBool(v)
		return true, nil
	case reflect.Float64, reflect.Float32:
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return true, fmt.Errorf("invalid float: %w", err)
		}
		elem.SetFloat(v)
		return true, nil
	}

	return false, nil
}
