// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sse

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	"github.com/lemon4ksan/sein"
)

var (
	ctxType       = reflect.TypeFor[context.Context]()
	sseWriterType = reflect.TypeFor[*Writer]()
	errorType     = reflect.TypeFor[error]()
)

// Handle compiles and registers a declarative Server-Sent Events (SSE) route onto router.
//
// Supported handler function signatures:
//   - func(w *sse.Writer) error
//   - func(ctx context.Context, w *sse.Writer) error
//   - func(w *sse.Writer, opt DTO) error
//   - func(ctx context.Context, w *sse.Writer, opt DTO) error
//
// When an incoming HTTP GET request arrives:
//  1. If the handler signature expects a DTO, request parameters/cookies/query are
//     bound into DTO and validated. If invalid, the request is rejected with 400 Bad Request
//     without opening the SSE stream.
//  2. HTTP 200 with text/event-stream headers is sent and flushed.
//  3. The handler function is executed with streaming writes.
func Handle(router sein.RouteBuilder, path string, fn any, mw ...sein.Middleware) {
	if fn == nil {
		panic(fmt.Sprintf("sse: route %q handler cannot be nil", path))
	}

	val := reflect.ValueOf(fn)
	typ := val.Type()

	if typ.Kind() != reflect.Func {
		panic(fmt.Sprintf("sse: route %q handler must be a function, got %T", path, fn))
	}

	numIn := typ.NumIn()
	numOut := typ.NumOut()

	if numOut != 1 || !typ.Out(0).Implements(errorType) {
		panic(fmt.Sprintf("sse: route %q handler must return error, got %d return values", path, numOut))
	}

	rawHandler := compileSSEHandler(val, typ, numIn, path)
	sein.Handle(router, http.MethodGet, path, rawHandler, mw...)
}

func compileSSEHandler(val reflect.Value, typ reflect.Type, numIn int, path string) sein.RawHandler {
	// 1. func(w *sse.Writer) error
	if numIn == 1 && typ.In(0) == sseWriterType {
		return func(req *sein.Request) (any, error) {
			return Stream(func(w *Writer) error {
				res := val.Call([]reflect.Value{reflect.ValueOf(w)})
				if errVal := res[0].Interface(); errVal != nil {
					return errVal.(error)
				}
				return nil
			}), nil
		}
	}

	// 2. func(ctx context.Context, w *sse.Writer) error
	if numIn == 2 && isContext(typ.In(0)) && typ.In(1) == sseWriterType {
		return func(req *sein.Request) (any, error) {
			ctx := req.Context()
			return Stream(func(w *Writer) error {
				res := val.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(w)})
				if errVal := res[0].Interface(); errVal != nil {
					return errVal.(error)
				}
				return nil
			}), nil
		}
	}

	// 3. func(w *sse.Writer, opt DTO) error
	if numIn == 2 && typ.In(0) == sseWriterType {
		dtoType := typ.In(1)
		isPtr := dtoType.Kind() == reflect.Pointer
		elemType := dtoType
		if isPtr {
			elemType = dtoType.Elem()
		}

		return func(req *sein.Request) (any, error) {
			destVal := reflect.New(elemType)
			if err := req.Bind(destVal.Interface()); err != nil {
				return nil, sein.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", err.Error())
			}

			var inVal reflect.Value
			if isPtr {
				inVal = destVal
			} else {
				inVal = destVal.Elem()
			}

			return Stream(func(w *Writer) error {
				res := val.Call([]reflect.Value{reflect.ValueOf(w), inVal})
				if errVal := res[0].Interface(); errVal != nil {
					return errVal.(error)
				}
				return nil
			}), nil
		}
	}

	// 4. func(ctx context.Context, w *sse.Writer, opt DTO) error
	if numIn == 3 && isContext(typ.In(0)) && typ.In(1) == sseWriterType {
		dtoType := typ.In(2)
		isPtr := dtoType.Kind() == reflect.Pointer
		elemType := dtoType
		if isPtr {
			elemType = dtoType.Elem()
		}

		return func(req *sein.Request) (any, error) {
			destVal := reflect.New(elemType)
			if err := req.Bind(destVal.Interface()); err != nil {
				return nil, sein.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", err.Error())
			}

			ctx := req.Context()
			var inVal reflect.Value
			if isPtr {
				inVal = destVal
			} else {
				inVal = destVal.Elem()
			}

			return Stream(func(w *Writer) error {
				res := val.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(w), inVal})
				if errVal := res[0].Interface(); errVal != nil {
					return errVal.(error)
				}
				return nil
			}), nil
		}
	}

	panic(fmt.Sprintf("sse: route %q handler signature %s is unsupported", path, typ.String()))
}

func isContext(t reflect.Type) bool {
	return t == ctxType || t.Implements(ctxType)
}
