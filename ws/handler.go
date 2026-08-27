// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	"github.com/lemon4ksan/sein"
)

var (
	ctxType    = reflect.TypeFor[context.Context]()
	wsConnType = reflect.TypeFor[*Conn]()
	reqPtrType = reflect.TypeFor[*sein.Request]()
	errorType  = reflect.TypeFor[error]()
)

// Handle compiles and registers a declarative WebSocket route onto router.
//
// Supported handler function signatures:
//   - func(conn *ws.Conn) error
//   - func(conn *ws.Conn, req *sein.Request) error
//   - func(conn *ws.Conn, opt DTO) error
//   - func(ctx context.Context, conn *ws.Conn) error
//   - func(ctx context.Context, conn *ws.Conn, opt DTO) error
//
// When an incoming HTTP GET request arrives:
//  1. If the handler signature expects a DTO, request parameters/cookies/query are
//     bound into DTO and validated. If invalid, the request is rejected with 400 Bad Request
//     without upgrading the socket.
//  2. The connection is upgraded to WebSocket according to RFC 6455.
//  3. The handler function is executed.
//  4. If the handler returns an error, the connection is automatically closed with the mapped
//     RFC 6455 status code via [ResolveCloseError] and [MapCloseError].
func Handle(router sein.RouteBuilder, path string, fn any, mw ...sein.Middleware) {
	if fn == nil {
		panic(fmt.Sprintf("ws: route %q handler cannot be nil", path))
	}

	val := reflect.ValueOf(fn)
	typ := val.Type()

	if typ.Kind() != reflect.Func {
		panic(fmt.Sprintf("ws: route %q handler must be a function, got %T", path, fn))
	}

	numIn := typ.NumIn()
	numOut := typ.NumOut()

	if numOut != 1 || !typ.Out(0).Implements(errorType) {
		panic(fmt.Sprintf("ws: route %q handler must return error, got %d return values", path, numOut))
	}

	rawHandler := compileHandler(val, typ, numIn, path)
	sein.Handle(router, http.MethodGet, path, rawHandler, mw...)
}

func compileHandler(val reflect.Value, typ reflect.Type, numIn int, path string) sein.RawHandler {
	// 1. func(conn *ws.Conn) error
	if numIn == 1 && typ.In(0) == wsConnType {
		return func(req *sein.Request) (any, error) {
			conn, err := Upgrade(req)
			if err != nil {
				return nil, err
			}
			defer conn.Close() //nolint:errcheck

			res := val.Call([]reflect.Value{reflect.ValueOf(conn)})
			if errVal := res[0].Interface(); errVal != nil {
				e := errVal.(error)
				code, reason := ResolveCloseError(e)
				_ = conn.CloseWithStatus(code, reason)
				return nil, nil
			}

			return nil, nil
		}
	}

	// 2. func(conn *ws.Conn, req *sein.Request) error
	if numIn == 2 && typ.In(0) == wsConnType && typ.In(1) == reqPtrType {
		return func(req *sein.Request) (any, error) {
			conn, err := Upgrade(req)
			if err != nil {
				return nil, err
			}
			defer conn.Close() //nolint:errcheck

			res := val.Call([]reflect.Value{reflect.ValueOf(conn), reflect.ValueOf(req)})
			if errVal := res[0].Interface(); errVal != nil {
				e := errVal.(error)
				code, reason := ResolveCloseError(e)
				_ = conn.CloseWithStatus(code, reason)
				return nil, nil
			}

			return nil, nil
		}
	}

	// 3. func(ctx context.Context, conn *ws.Conn) error
	if numIn == 2 && isContext(typ.In(0)) && typ.In(1) == wsConnType {
		return func(req *sein.Request) (any, error) {
			conn, err := Upgrade(req)
			if err != nil {
				return nil, err
			}
			defer conn.Close() //nolint:errcheck

			res := val.Call([]reflect.Value{reflect.ValueOf(req.Context()), reflect.ValueOf(conn)})
			if errVal := res[0].Interface(); errVal != nil {
				e := errVal.(error)
				code, reason := ResolveCloseError(e)
				_ = conn.CloseWithStatus(code, reason)
				return nil, nil
			}

			return nil, nil
		}
	}

	// 4. func(conn *ws.Conn, opt DTO) error
	if numIn == 2 && typ.In(0) == wsConnType {
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

			conn, err := Upgrade(req)
			if err != nil {
				return nil, err
			}
			defer conn.Close() //nolint:errcheck

			var inVal reflect.Value
			if isPtr {
				inVal = destVal
			} else {
				inVal = destVal.Elem()
			}

			res := val.Call([]reflect.Value{reflect.ValueOf(conn), inVal})
			if errVal := res[0].Interface(); errVal != nil {
				e := errVal.(error)
				code, reason := ResolveCloseError(e)
				_ = conn.CloseWithStatus(code, reason)
				return nil, nil
			}

			return nil, nil
		}
	}

	// 5. func(ctx context.Context, conn *ws.Conn, opt DTO) error
	if numIn == 3 && isContext(typ.In(0)) && typ.In(1) == wsConnType {
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

			conn, err := Upgrade(req)
			if err != nil {
				return nil, err
			}
			defer conn.Close() //nolint:errcheck

			var inVal reflect.Value
			if isPtr {
				inVal = destVal
			} else {
				inVal = destVal.Elem()
			}

			res := val.Call([]reflect.Value{reflect.ValueOf(req.Context()), reflect.ValueOf(conn), inVal})
			if errVal := res[0].Interface(); errVal != nil {
				e := errVal.(error)
				code, reason := ResolveCloseError(e)
				_ = conn.CloseWithStatus(code, reason)
				return nil, nil
			}

			return nil, nil
		}
	}

	panic(fmt.Sprintf("ws: route %q handler signature %s is unsupported", path, typ.String()))
}

func isContext(t reflect.Type) bool {
	return t == ctxType || t.Implements(ctxType)
}
