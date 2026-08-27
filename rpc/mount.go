// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/lemon4ksan/sein"
)

var (
	contextType = reflect.TypeFor[context.Context]()
	errorType   = reflect.TypeFor[error]()
)

// Mount reflects over all exported methods of a service receiver struct and mounts them
// as strongly-typed JSON-RPC POST endpoints on the router under the given prefix.
//
// Supported service method signatures:
//   - func(s *Service, ctx context.Context, in Req) (Res, error)
//   - func(s *Service, in Req) (Res, error)
//   - func(s *Service, ctx context.Context) (Res, error)
//   - func(s *Service) (Res, error)
//
// Each matching method is registered as `POST /prefix/MethodName`.
// Request bodies are automatically bound into `Req` with validation before method invocation.
func Mount(router sein.RouteBuilder, prefix string, service any, mw ...sein.Middleware) {
	if service == nil {
		panic("rpc: service cannot be nil")
	}

	val := reflect.ValueOf(service)
	typ := val.Type()

	if typ.Kind() != reflect.Pointer && typ.Kind() != reflect.Struct {
		panic(fmt.Sprintf("rpc: service must be a struct or pointer to struct, got %T", service))
	}

	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		mTyp := method.Type

		numIn := mTyp.NumIn()   // includes receiver at index 0
		numOut := mTyp.NumOut()

		if numOut != 2 || !mTyp.Out(1).Implements(errorType) {
			continue
		}

		handler := compileMethod(val, method, mTyp, numIn)
		if handler == nil {
			continue
		}

		cleanPrefix := "/" + strings.Trim(prefix, "/")
		path := cleanPrefix + "/" + method.Name

		sein.Handle(router, http.MethodPost, path, handler, mw...)
	}
}

func compileMethod(receiver reflect.Value, method reflect.Method, mTyp reflect.Type, numIn int) sein.RawHandler {
	// Case 1: func(s *Service) (Res, error)
	if numIn == 1 {
		return func(req *sein.Request) (any, error) {
			results := receiver.Method(method.Index).Call(nil)
			if errVal := results[1].Interface(); errVal != nil {
				return nil, errVal.(error)
			}
			return results[0].Interface(), nil
		}
	}

	// Case 2: func(s *Service, ctx context.Context) (Res, error)
	if numIn == 2 && isCtx(mTyp.In(1)) {
		return func(req *sein.Request) (any, error) {
			results := receiver.Method(method.Index).Call([]reflect.Value{
				reflect.ValueOf(req.Context()),
			})
			if errVal := results[1].Interface(); errVal != nil {
				return nil, errVal.(error)
			}
			return results[0].Interface(), nil
		}
	}

	// Case 3: func(s *Service, in Req) (Res, error)
	if numIn == 2 && !isCtx(mTyp.In(1)) {
		reqType := mTyp.In(1)
		isPtr := reqType.Kind() == reflect.Pointer
		elemType := reqType
		if isPtr {
			elemType = reqType.Elem()
		}

		return func(req *sein.Request) (any, error) {
			destVal := reflect.New(elemType)
			if err := req.Bind(destVal.Interface()); err != nil {
				return nil, sein.BadRequest("BAD_REQUEST", err.Error())
			}

			var inVal reflect.Value
			if isPtr {
				inVal = destVal
			} else {
				inVal = destVal.Elem()
			}

			results := receiver.Method(method.Index).Call([]reflect.Value{inVal})
			if errVal := results[1].Interface(); errVal != nil {
				return nil, errVal.(error)
			}
			return results[0].Interface(), nil
		}
	}

	// Case 4: func(s *Service, ctx context.Context, in Req) (Res, error)
	if numIn == 3 && isCtx(mTyp.In(1)) {
		reqType := mTyp.In(2)
		isPtr := reqType.Kind() == reflect.Pointer
		elemType := reqType
		if isPtr {
			elemType = reqType.Elem()
		}

		return func(req *sein.Request) (any, error) {
			destVal := reflect.New(elemType)
			if err := req.Bind(destVal.Interface()); err != nil {
				return nil, sein.BadRequest("BAD_REQUEST", err.Error())
			}

			var inVal reflect.Value
			if isPtr {
				inVal = destVal
			} else {
				inVal = destVal.Elem()
			}

			results := receiver.Method(method.Index).Call([]reflect.Value{
				reflect.ValueOf(req.Context()),
				inVal,
			})
			if errVal := results[1].Interface(); errVal != nil {
				return nil, errVal.(error)
			}
			return results[0].Interface(), nil
		}
	}

	return nil
}

func isCtx(t reflect.Type) bool {
	return t == contextType || t.Implements(contextType)
}
