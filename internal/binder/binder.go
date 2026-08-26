// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"errors"
	"reflect"
	"unsafe"
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

// Ingest executes the precompiled field steps across dest with zero runtime switch or allocations.
func Ingest[T any](req RequestView, dest *T) error {
	if dest == nil {
		return errors.New("dest must be a non-nil pointer to struct")
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

	return RunValidation(dest)
}
