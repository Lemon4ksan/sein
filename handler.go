// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"slices"
	"context"
	"net/http"
)

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
