// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"errors"
	"reflect"
)

// Common error sentinels returned by binder engine.
var (
	ErrMissingPathParam   = errors.New("required path parameter is missing")
	ErrMissingQueryParam  = errors.New("required query parameter is missing")
	ErrMissingHeader      = errors.New("required header is missing")
	ErrMissingCookie      = errors.New("required cookie is missing")
	ErrMissingBearerToken = errors.New("authorization bearer token is required")
	ErrMissingContext     = errors.New("required context value is missing")
	ErrEmptyRequestBody   = errors.New("request body cannot be empty")
)

// Ingest executes the precompiled 4-stage pipeline across all fields in dest.
func Ingest(req RequestView, dest any) error {
	// 1. Check Ingestable interface (vortex gen compiled fast-path)
	if ing, ok := dest.(Ingestable); ok {
		if err := ing.IngestAny(req); err != nil {
			return err
		}
		return RunValidation(dest)
	}

	val := reflect.ValueOf(dest)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return errors.New("dest must be a non-nil pointer to struct")
	}

	typ := val.Type().Elem()
	desc := GetDescriptor(typ)
	if desc == nil {
		if len(req.Body()) > 0 {
			if err := req.BindJSON(dest); err != nil {
				return err
			}
		}
		return RunValidation(dest)
	}

	ptr := val.UnsafePointer()

	// 2. Linear execution of precompiled field pipelines (0-switch, 0-alloc overhead)
	for i := range desc.Pipelines {
		if err := desc.Pipelines[i].Execute(req, ptr); err != nil {
			return err
		}
	}

	// 3. Extract JSON body if fields are defined
	if desc.HasBodyFields && len(req.Body()) > 0 {
		if err := req.BindJSON(dest); err != nil {
			return err
		}
	}

	return RunValidation(dest)
}
