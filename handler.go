// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
	"net/http"
)

// Handle registers a raw handler function on any RouteBuilder (Server or Group).
func Handle(r RouteBuilder, method, path string, fn RawHandler, mw ...Middleware) {
	if srv, ok := r.(*Server); ok {
		handler := fn
		for i := len(mw) - 1; i >= 0; i-- {
			handler = mw[i](handler)
		}
		srv.router.Add(method, path, handler)
		return
	}
	r.registerRoute(method, path, fn, mw...)
}

// GET registers a pure GET handler: (context.Context) -> (Res, error)
func GET[Res any](r RouteBuilder, path string, fn func(ctx context.Context) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodGet, path, func(req *Request) (any, error) {
		res, err := fn(req.Context())
		if err != nil {
			return nil, err
		}
		return toResponder(res), nil
	}, mw...)
}

// GETReq registers a GET handler with Request metadata: (*Request) -> (Res, error)
func GETReq[Res any](r RouteBuilder, path string, fn func(req *Request) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodGet, path, func(req *Request) (any, error) {
		res, err := fn(req)
		if err != nil {
			return nil, err
		}
		return toResponder(res), nil
	}, mw...)
}

// POST registers a pure POST handler: (context.Context, Req) -> (Res, error)
// The request body is automatically decoded into Req before calling fn.
func POST[Req, Res any](r RouteBuilder, path string, fn func(ctx context.Context, body Req) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodPost, path, func(req *Request) (any, error) {
		var body Req
		if err := req.BindJSON(&body); err != nil {
			return nil, err
		}
		res, err := fn(req.Context(), body)
		if err != nil {
			return nil, err
		}
		return toResponder(res), nil
	}, mw...)
}

// POSTReq registers a POST handler with Request metadata: (*Request, Req) -> (Res, error)
func POSTReq[Req, Res any](r RouteBuilder, path string, fn func(req *Request, body Req) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodPost, path, func(req *Request) (any, error) {
		var body Req
		if err := req.BindJSON(&body); err != nil {
			return nil, err
		}
		res, err := fn(req, body)
		if err != nil {
			return nil, err
		}
		return toResponder(res), nil
	}, mw...)
}

// PUT registers a pure PUT handler: (context.Context, Req) -> (Res, error)
func PUT[Req, Res any](r RouteBuilder, path string, fn func(ctx context.Context, body Req) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodPut, path, func(req *Request) (any, error) {
		var body Req
		if err := req.BindJSON(&body); err != nil {
			return nil, err
		}
		res, err := fn(req.Context(), body)
		if err != nil {
			return nil, err
		}
		return toResponder(res), nil
	}, mw...)
}

// PUTReq registers a PUT handler with Request metadata: (*Request, Req) -> (Res, error)
func PUTReq[Req, Res any](r RouteBuilder, path string, fn func(req *Request, body Req) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodPut, path, func(req *Request) (any, error) {
		var body Req
		if err := req.BindJSON(&body); err != nil {
			return nil, err
		}
		res, err := fn(req, body)
		if err != nil {
			return nil, err
		}
		return toResponder(res), nil
	}, mw...)
}

// PATCH registers a pure PATCH handler: (context.Context, Req) -> (Res, error)
func PATCH[Req, Res any](r RouteBuilder, path string, fn func(ctx context.Context, body Req) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodPatch, path, func(req *Request) (any, error) {
		var body Req
		if err := req.BindJSON(&body); err != nil {
			return nil, err
		}
		res, err := fn(req.Context(), body)
		if err != nil {
			return nil, err
		}
		return toResponder(res), nil
	}, mw...)
}

// PATCHReq registers a PATCH handler with Request metadata: (*Request, Req) -> (Res, error)
func PATCHReq[Req, Res any](r RouteBuilder, path string, fn func(req *Request, body Req) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodPatch, path, func(req *Request) (any, error) {
		var body Req
		if err := req.BindJSON(&body); err != nil {
			return nil, err
		}
		res, err := fn(req, body)
		if err != nil {
			return nil, err
		}
		return toResponder(res), nil
	}, mw...)
}

// DELETE registers a pure DELETE handler: (context.Context) -> (Res, error)
func DELETE[Res any](r RouteBuilder, path string, fn func(ctx context.Context) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodDelete, path, func(req *Request) (any, error) {
		res, err := fn(req.Context())
		if err != nil {
			return nil, err
		}
		return toResponder(res), nil
	}, mw...)
}

// DELETEReq registers a DELETE handler with Request metadata: (*Request) -> (Res, error)
func DELETEReq[Res any](r RouteBuilder, path string, fn func(req *Request) (Res, error), mw ...Middleware) {
	Handle(r, http.MethodDelete, path, func(req *Request) (any, error) {
		res, err := fn(req)
		if err != nil {
			return nil, err
		}
		return toResponder(res), nil
	}, mw...)
}

func toResponder[T any](val T) any {
	if responder, ok := any(val).(Responder); ok {
		return responder
	}
	return OK(val)
}
