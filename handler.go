// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"slices"

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

			var inVal reflect.Value
			if isPtr {
				inVal = destVal
			} else {
				inVal = destVal.Elem()
			}

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

			var p1In reflect.Value
			if p1IsPtr {
				p1In = p1Dest
			} else {
				p1In = p1Dest.Elem()
			}

			// Bind P2 from body/query
			p2Dest := reflect.New(p2ElemType)
			if err := binder.IngestType(adapter, p2ElemType, p2Dest.Interface()); err != nil {
				return nil, mapBinderError(err)
			}

			var p2In reflect.Value
			if p2IsPtr {
				p2In = p2Dest
			} else {
				p2In = p2Dest.Elem()
			}

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

			var inVal reflect.Value
			if isPtr {
				inVal = destVal
			} else {
				inVal = destVal.Elem()
			}

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
	Handle(r, method, path, compiled, mw...)
}

// Handle registers a raw handler function on any RouteBuilder (Server or Group).
func Handle(r RouteBuilder, method, path string, fn RawHandler, mw ...Middleware) {
	if srv, ok := r.(*Server); ok {
		handler := fn
		for _, m := range slices.Backward(mw) {
			handler = m(handler)
		}

		srv.router.Add(method, path, handler)

		return
	}

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

	return OK(val)
}

// routeGet registers a pure GET handler without parameters: (context.Context) -> (Res, error)
func routeGet[Res any](r RouteBuilder, path string, fn func(ctx context.Context) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodGet, path, func(req *Request) (any, error) {
		res, err := fn(req.Context())
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routeGetWith registers a pure GET handler with Path/Query/Header DTO: (context.Context, Req) -> (Res, error)
func routeGetWith[Req, Res any](
	r RouteBuilder,
	path string,
	fn func(ctx context.Context, req Req) (Res, error),
	mw ...Middleware,
) {
	ValidateRouteBinding[Req](path)
	Handle(r, http.MethodGet, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}

		res, err := fn(req.Context(), body)
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routeGetReq registers a GET handler with Request metadata: (*Request) -> (Res, error)
func routeGetReq[Res any](r RouteBuilder, path string, fn func(req *Request) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodGet, path, func(req *Request) (any, error) {
		res, err := fn(req)
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routePost registers a pure POST handler: (context.Context, Req) -> (Res, error)
func routePost[Req, Res any](
	r RouteBuilder,
	path string,
	fn func(ctx context.Context, body Req) (Res, error),
	mw ...Middleware,
) {
	ValidateRouteBinding[Req](path)
	Handle(r, http.MethodPost, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}

		res, err := fn(req.Context(), body)
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routePostAction registers a pure parameterless POST handler: (context.Context) -> (Res, error)
func routePostAction[Res any](
	r RouteBuilder,
	path string,
	fn func(ctx context.Context) (Res, error),
	mw ...Middleware,
) {
	Handle(r, http.MethodPost, path, func(req *Request) (any, error) {
		res, err := fn(req.Context())
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routePostReq registers a POST handler with Request metadata: (*Request, Req) -> (Res, error)
func routePostReq[Req, Res any](
	r RouteBuilder,
	path string,
	fn func(req *Request, body Req) (Res, error),
	mw ...Middleware,
) {
	ValidateRouteBinding[Req](path)
	Handle(r, http.MethodPost, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}

		res, err := fn(req, body)
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routePut registers a pure PUT handler: (context.Context, Req) -> (Res, error)
func routePut[Req, Res any](
	r RouteBuilder,
	path string,
	fn func(ctx context.Context, body Req) (Res, error),
	mw ...Middleware,
) {
	ValidateRouteBinding[Req](path)
	Handle(r, http.MethodPut, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}

		res, err := fn(req.Context(), body)
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routePutReq registers a PUT handler with Request metadata: (*Request, Req) -> (Res, error)
func routePutReq[Req, Res any](
	r RouteBuilder,
	path string,
	fn func(req *Request, body Req) (Res, error),
	mw ...Middleware,
) {
	ValidateRouteBinding[Req](path)
	Handle(r, http.MethodPut, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}

		res, err := fn(req, body)
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routePutAction registers a pure parameterless PUT handler: (context.Context) -> (Res, error)
func routePutAction[Res any](r RouteBuilder, path string, fn func(ctx context.Context) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodPut, path, func(req *Request) (any, error) {
		res, err := fn(req.Context())
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routePatchAction registers a pure parameterless PATCH handler: (context.Context) -> (Res, error)
func routePatchAction[Res any](r RouteBuilder, path string, fn func(ctx context.Context) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodPatch, path, func(req *Request) (any, error) {
		res, err := fn(req.Context())
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routePatch registers a pure PATCH handler: (context.Context, Req) -> (Res, error)
func routePatch[Req, Res any](
	r RouteBuilder,
	path string,
	fn func(ctx context.Context, body Req) (Res, error),
	mw ...Middleware,
) {
	ValidateRouteBinding[Req](path)
	Handle(r, http.MethodPatch, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}

		res, err := fn(req.Context(), body)
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routePatchReq registers a PATCH handler with Request metadata: (*Request, Req) -> (Res, error)
func routePatchReq[Req, Res any](
	r RouteBuilder,
	path string,
	fn func(req *Request, body Req) (Res, error),
	mw ...Middleware,
) {
	ValidateRouteBinding[Req](path)
	Handle(r, http.MethodPatch, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}

		res, err := fn(req, body)
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routeDelete registers a pure DELETE handler without parameters: (context.Context) -> (Res, error)
func routeDelete[Res any](r RouteBuilder, path string, fn func(ctx context.Context) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodDelete, path, func(req *Request) (any, error) {
		res, err := fn(req.Context())
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routeDeleteWith registers a pure DELETE handler with Path/Query DTO: (context.Context, Req) -> (Res, error)
func routeDeleteWith[Req, Res any](
	r RouteBuilder,
	path string,
	fn func(ctx context.Context, req Req) (Res, error),
	mw ...Middleware,
) {
	ValidateRouteBinding[Req](path)
	Handle(r, http.MethodDelete, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}

		res, err := fn(req.Context(), body)
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}

// routeDeleteReq registers a DELETE handler with Request metadata: (*Request) -> (Res, error)
func routeDeleteReq[Res any](r RouteBuilder, path string, fn func(req *Request) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodDelete, path, func(req *Request) (any, error) {
		res, err := fn(req)
		if err != nil {
			return nil, err
		}

		return toResponder(res), nil
	}, mw...)
}
