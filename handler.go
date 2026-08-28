// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
	"fmt"
	"iter"
	"reflect"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/sein/internal/binder"
)

var (
	ctxInterfaceType = reflect.TypeFor[context.Context]()
	reqPtrType       = reflect.TypeFor[*Request]()
	errorType        = reflect.TypeFor[error]()
)

func isContext(t reflect.Type) bool {
	return t == ctxInterfaceType || t.Implements(ctxInterfaceType)
}

func compileUniversalHandler(fn any, routePath string) RawHandler {
	if fn == nil {
		panic(fmt.Sprintf("sein: route %q handler cannot be nil", routePath))
	}

	if raw, ok := fn.(RawHandler); ok {
		return raw
	}
	if rawFunc, ok := fn.(func(*Request) (any, error)); ok {
		return rawFunc
	}

	val := reflect.ValueOf(fn)
	typ := val.Type()

	if typ.Kind() != reflect.Func {
		panic(fmt.Sprintf("sein: route %q handler must be a function, got %T", routePath, fn))
	}

	numIn := typ.NumIn()
	numOut := typ.NumOut()

	hasRes := numOut == 2
	hasErrOnly := numOut == 1

	if !hasRes && !hasErrOnly {
		panic(fmt.Sprintf("sein: route %q handler must return (Res, error) or error, got %d return values", routePath, numOut))
	}

	lastOut := typ.Out(numOut - 1)
	if !lastOut.Implements(errorType) {
		panic(fmt.Sprintf("sein: route %q handler last return value must be error, got %s", routePath, lastOut.String()))
	}

	// 1. func(context.Context) (Res, error) OR func(context.Context) error
	if numIn == 1 && isContext(typ.In(0)) {
		return func(req *Request) (any, error) {
			results := val.Call([]reflect.Value{reflect.ValueOf(req.Context())})
			if hasRes {
				if errVal := results[1].Interface(); errVal != nil {
					return nil, errVal.(error)
				}
				return toResponder(results[0].Interface()), nil
			}
			if errVal := results[0].Interface(); errVal != nil {
				return nil, errVal.(error)
			}
			return OK("OK"), nil
		}
	}

	// 2. func(*Request) (Res, error) OR func(*Request) error
	if numIn == 1 && typ.In(0) == reqPtrType {
		return func(req *Request) (any, error) {
			results := val.Call([]reflect.Value{reflect.ValueOf(req)})
			if hasRes {
				if errVal := results[1].Interface(); errVal != nil {
					return nil, errVal.(error)
				}
				return toResponder(results[0].Interface()), nil
			}
			if errVal := results[0].Interface(); errVal != nil {
				return nil, errVal.(error)
			}
			return OK("OK"), nil
		}
	}

	// 3. func(context.Context, Req) (Res, error) OR func(context.Context, Req) error
	if numIn == 2 && isContext(typ.In(0)) {
		pType := typ.In(1)
		isPtr := pType.Kind() == reflect.Pointer
		elemType := pType
		if isPtr {
			elemType = pType.Elem()
		}

		ValidateRouteBindingType(elemType, routePath)

		return func(req *Request) (any, error) {
			destVal := reflect.New(elemType)
			adapter := newRequestAdapter(req)
			if err := binder.IngestType(adapter, elemType, destVal.Interface()); err != nil {
				return nil, mapBinderError(err)
			}

			inVal := generic.Ternary(isPtr, destVal, destVal.Elem())

			results := val.Call([]reflect.Value{
				reflect.ValueOf(req.Context()),
				inVal,
			})

			if hasRes {
				if errVal := results[1].Interface(); errVal != nil {
					return nil, errVal.(error)
				}
				return toResponder(results[0].Interface()), nil
			}

			if errVal := results[0].Interface(); errVal != nil {
				return nil, errVal.(error)
			}
			return OK("OK"), nil
		}
	}

	// 4. func(context.Context, P1, P2) (Res, error) OR func(context.Context, P1, P2) error
	// (e.g. func(ctx, id Snowflake, payload UpdatePayload))
	if numIn == 3 && isContext(typ.In(0)) {
		p1Type := typ.In(1)
		p2Type := typ.In(2)

		p1IsPtr := p1Type.Kind() == reflect.Pointer
		p1ElemType := p1Type
		if p1IsPtr {
			p1ElemType = p1Type.Elem()
		}

		p2IsPtr := p2Type.Kind() == reflect.Pointer
		p2ElemType := p2Type
		if p2IsPtr {
			p2ElemType = p2Type.Elem()
		}

		return func(req *Request) (any, error) {
			adapter := newRequestAdapter(req)

			// Bind P1 from scalar path param
			p1Dest := reflect.New(p1ElemType)
			if err := binder.IngestScalarType(adapter, p1ElemType, p1Dest.Interface()); err != nil {
				return nil, mapBinderError(err)
			}

			p1In := generic.Ternary(p1IsPtr, p1Dest, p1Dest.Elem())

			// Bind P2 from body/query
			p2Dest := reflect.New(p2ElemType)
			if err := binder.IngestType(adapter, p2ElemType, p2Dest.Interface()); err != nil {
				return nil, mapBinderError(err)
			}

			p2In := generic.Ternary(p2IsPtr, p2Dest, p2Dest.Elem())

			results := val.Call([]reflect.Value{
				reflect.ValueOf(req.Context()),
				p1In,
				p2In,
			})

			if hasRes {
				if errVal := results[1].Interface(); errVal != nil {
					return nil, errVal.(error)
				}
				return toResponder(results[0].Interface()), nil
			}

			if errVal := results[0].Interface(); errVal != nil {
				return nil, errVal.(error)
			}
			return OK("OK"), nil
		}
	}

	// 5. func(*Request, Req) (Res, error) OR func(*Request, Req) error
	if numIn == 2 && typ.In(0) == reqPtrType {
		pType := typ.In(1)
		isPtr := pType.Kind() == reflect.Pointer
		elemType := pType
		if isPtr {
			elemType = pType.Elem()
		}

		ValidateRouteBindingType(elemType, routePath)

		return func(req *Request) (any, error) {
			destVal := reflect.New(elemType)
			adapter := newRequestAdapter(req)
			if err := binder.IngestType(adapter, elemType, destVal.Interface()); err != nil {
				return nil, mapBinderError(err)
			}

			inVal := generic.Ternary(isPtr, destVal, destVal.Elem())

			results := val.Call([]reflect.Value{
				reflect.ValueOf(req),
				inVal,
			})

			if hasRes {
				if errVal := results[1].Interface(); errVal != nil {
					return nil, errVal.(error)
				}
				return toResponder(results[0].Interface()), nil
			}

			if errVal := results[0].Interface(); errVal != nil {
				return nil, errVal.(error)
			}
			return OK("OK"), nil
		}
	}

	panic(fmt.Sprintf("sein: unsupported handler signature %T for route %q", fn, routePath))
}

func routeUniversal(r RouteBuilder, method, path string, handler any, mw ...Middleware) {
	compiled := compileUniversalHandler(handler, path)
	ht := generic.Ternary(handler != nil, reflect.TypeOf(handler), nil)
	r.registerRouteWithType(method, path, compiled, ht, mw...)
}

// Handle registers a raw handler function on any RouteBuilder (Server or Group).
func Handle(r RouteBuilder, method, path string, fn RawHandler, mw ...Middleware) {
	r.registerRoute(method, path, fn, mw...)
}

func bindAndValidate[Req any](req *Request) (Req, error) {
	var body Req
	if err := IngestDTO(req, &body); err != nil {
		return body, err
	}

	return body, nil
}

func toResponder[T any](val T) any {
	if responder, ok := any(val).(Responder); ok {
		return responder
	}
	if direct, ok := any(val).(DirectH1Responder); ok {
		return direct
	}
	if rVal := reflect.ValueOf(val); rVal.IsValid() {
		rTyp := rVal.Type()
		if isIterSeq(rTyp) {
			return Stream(makeIterSeq(rVal))
		}
		if isChannel(rTyp) {
			return Stream(makeChanSeq(rVal))
		}
	}

	return OK(val)
}

func isIterSeq(t reflect.Type) bool {
	if t == nil || t.Kind() != reflect.Func || t.NumIn() != 1 || t.NumOut() != 0 {
		return false
	}
	yieldType := t.In(0)
	if yieldType.Kind() != reflect.Func || yieldType.NumIn() != 1 || yieldType.NumOut() != 1 {
		return false
	}
	return yieldType.Out(0).Kind() == reflect.Bool
}

func makeIterSeq(val reflect.Value) iter.Seq[any] {
	return func(yield func(any) bool) {
		yieldVal := reflect.MakeFunc(val.Type().In(0), func(args []reflect.Value) []reflect.Value {
			item := args[0].Interface()
			cont := yield(item)
			return []reflect.Value{reflect.ValueOf(cont)}
		})
		val.Call([]reflect.Value{yieldVal})
	}
}

func isChannel(t reflect.Type) bool {
	return t != nil && t.Kind() == reflect.Chan
}

func makeChanSeq(val reflect.Value) iter.Seq[any] {
	return func(yield func(any) bool) {
		for {
			chosen, recv, ok := reflect.Select([]reflect.SelectCase{
				{Dir: reflect.SelectRecv, Chan: val},
			})
			_ = chosen
			if !ok {
				return
			}
			if !yield(recv.Interface()) {
				return
			}
		}
	}
}
