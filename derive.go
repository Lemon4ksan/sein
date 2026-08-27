// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
)

// ResolverFunc extracts a strongly-typed value T from an incoming HTTP request.
// If resolution fails (e.g. invalid JWT, missing session, expired token), the returned error
// is automatically written to the HTTP response and handler execution is aborted.
type ResolverFunc[T any] func(req *Request) (T, error)

// Derive registers a request-scoped type resolver for type T on the provided server.
func Derive[T any](s *Server, fn ResolverFunc[T]) *Server {
	t := reflect.TypeOf((*T)(nil)).Elem()
	s.resolvers.Store(t, fn)
	return s
}

// Provide is an alias for [Derive] to register a request-scoped dependency provider.
func Provide[T any](s *Server, fn ResolverFunc[T]) *Server {
	return Derive[T](s, fn)
}

// RegisterResolver is an alias for [Derive].
func RegisterResolver[T any](s *Server, fn ResolverFunc[T]) *Server {
	return Derive[T](s, fn)
}

// DeriveMiddleware creates a middleware that resolves dependency T and injects it into the request context.
func DeriveMiddleware[T any](fn ResolverFunc[T]) Middleware {
	t := reflect.TypeOf((*T)(nil)).Elem()
	return func(next RawHandler) RawHandler {
		return func(req *Request) (any, error) {
			val, err := fn(req)
			if err != nil {
				return nil, err
			}
			req.ctx = context.WithValue(req.Context(), t, val)
			return next(req)
		}
	}
}

// ProvideMiddleware is an alias for [DeriveMiddleware].
func ProvideMiddleware[T any](fn ResolverFunc[T]) Middleware {
	return DeriveMiddleware[T](fn)
}

func findServer(r RouteBuilder) *Server {
	if srv, ok := r.(*Server); ok {
		return srv
	}
	if g, ok := r.(*Group); ok {
		return findServer(g.parent)
	}
	if gs, ok := r.(*GuardScope); ok {
		return findServer(gs.Group)
	}
	return nil
}

func getResolver[T any](r RouteBuilder) ResolverFunc[T] {
	srv := findServer(r)
	if srv != nil {
		t := reflect.TypeOf((*T)(nil)).Elem()
		if raw, ok := srv.resolvers.Load(t); ok {
			if fn, ok := raw.(ResolverFunc[T]); ok {
				return fn
			}
		}
	}

	return func(req *Request) (T, error) {
		ctxVal := req.Context().Value(reflect.TypeOf((*T)(nil)).Elem())
		if ctxVal != nil {
			if typed, ok := ctxVal.(T); ok {
				return typed, nil
			}
		}

		var zero T
		return zero, Unauthorized("UNAUTHORIZED", fmt.Sprintf("unresolved context dependency: %T", zero))
	}
}

// WithValue injects a typed value into a context so it can be resolved by type-directed handlers.
func WithValue[T any](ctx context.Context, val T) context.Context {
	return context.WithValue(ctx, reflect.TypeOf((*T)(nil)).Elem(), val)
}

// Route Registration Overloads with Type-Directed Authentication / Extractor Parameters

// GetAuth registers a GET handler: (ctx, Auth) -> (Res, error)
func (s *Server) GetAuth[Res, Auth any](path string, fn func(context.Context, Auth) (Res, error), mw ...Middleware) {
	routeGetAuth(s, path, fn, mw...)
}

// GetWithAuth registers a GET handler with request DTO and Auth: (ctx, Req, Auth) -> (Res, error)
func (s *Server) GetWithAuth[Req, Res, Auth any](path string, fn func(context.Context, Req, Auth) (Res, error), mw ...Middleware) {
	routeGetWithAuth(s, path, fn, mw...)
}

// PostAuth registers a POST handler: (ctx, Req, Auth) -> (Res, error)
func (s *Server) PostAuth[Req, Res, Auth any](path string, fn func(context.Context, Req, Auth) (Res, error), mw ...Middleware) {
	routePostAuth(s, path, fn, mw...)
}

// PutAuth registers a PUT handler: (ctx, Req, Auth) -> (Res, error)
func (s *Server) PutAuth[Req, Res, Auth any](path string, fn func(context.Context, Req, Auth) (Res, error), mw ...Middleware) {
	routePutAuth(s, path, fn, mw...)
}

// PatchAuth registers a PATCH handler: (ctx, Req, Auth) -> (Res, error)
func (s *Server) PatchAuth[Req, Res, Auth any](path string, fn func(context.Context, Req, Auth) (Res, error), mw ...Middleware) {
	routePatchAuth(s, path, fn, mw...)
}

// DeleteAuth registers a DELETE handler: (ctx, Auth) -> (Res, error)
func (s *Server) DeleteAuth[Res, Auth any](path string, fn func(context.Context, Auth) (Res, error), mw ...Middleware) {
	routeDeleteAuth(s, path, fn, mw...)
}

// DeleteWithAuth registers a DELETE handler with request DTO and Auth: (ctx, Req, Auth) -> (Res, error)
func (s *Server) DeleteWithAuth[Req, Res, Auth any](path string, fn func(context.Context, Req, Auth) (Res, error), mw ...Middleware) {
	routeDeleteWithAuth(s, path, fn, mw...)
}

// Group methods

// GetAuth registers a GET handler on a group: (ctx, Auth) -> (Res, error)
func (g *Group) GetAuth[Res, Auth any](path string, fn func(context.Context, Auth) (Res, error), mw ...Middleware) {
	routeGetAuth(g, path, fn, mw...)
}

// GetWithAuth registers a GET handler on a group with request DTO and Auth: (ctx, Req, Auth) -> (Res, error)
func (g *Group) GetWithAuth[Req, Res, Auth any](path string, fn func(context.Context, Req, Auth) (Res, error), mw ...Middleware) {
	routeGetWithAuth(g, path, fn, mw...)
}

// PostAuth registers a POST handler on a group: (ctx, Req, Auth) -> (Res, error)
func (g *Group) PostAuth[Req, Res, Auth any](path string, fn func(context.Context, Req, Auth) (Res, error), mw ...Middleware) {
	routePostAuth(g, path, fn, mw...)
}

// PutAuth registers a PUT handler on a group: (ctx, Req, Auth) -> (Res, error)
func (g *Group) PutAuth[Req, Res, Auth any](path string, fn func(context.Context, Req, Auth) (Res, error), mw ...Middleware) {
	routePutAuth(g, path, fn, mw...)
}

// PatchAuth registers a PATCH handler on a group: (ctx, Req, Auth) -> (Res, error)
func (g *Group) PatchAuth[Req, Res, Auth any](path string, fn func(context.Context, Req, Auth) (Res, error), mw ...Middleware) {
	routePatchAuth(g, path, fn, mw...)
}

// DeleteAuth registers a DELETE handler on a group: (ctx, Auth) -> (Res, error)
func (g *Group) DeleteAuth[Res, Auth any](path string, fn func(context.Context, Auth) (Res, error), mw ...Middleware) {
	routeDeleteAuth(g, path, fn, mw...)
}

// DeleteWithAuth registers a DELETE handler on a group with request DTO and Auth: (ctx, Req, Auth) -> (Res, error)
func (g *Group) DeleteWithAuth[Req, Res, Auth any](path string, fn func(context.Context, Req, Auth) (Res, error), mw ...Middleware) {
	routeDeleteWithAuth(g, path, fn, mw...)
}

// Internal routing implementations

func routeGetAuth[Res, Auth any](
	r RouteBuilder,
	path string,
	fn func(context.Context, Auth) (Res, error),
	mw ...Middleware,
) {
	resolver := getResolver[Auth](r)
	Handle(r, http.MethodGet, path, func(req *Request) (any, error) {
		auth, err := resolver(req)
		if err != nil {
			return nil, err
		}
		return fn(req.Context(), auth)
	}, mw...)
}

func routeGetWithAuth[Req, Res, Auth any](
	r RouteBuilder,
	path string,
	fn func(context.Context, Req, Auth) (Res, error),
	mw ...Middleware,
) {
	resolver := getResolver[Auth](r)
	Handle(r, http.MethodGet, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}
		auth, err := resolver(req)
		if err != nil {
			return nil, err
		}
		return fn(req.Context(), body, auth)
	}, mw...)
}

func routePostAuth[Req, Res, Auth any](
	r RouteBuilder,
	path string,
	fn func(context.Context, Req, Auth) (Res, error),
	mw ...Middleware,
) {
	resolver := getResolver[Auth](r)
	Handle(r, http.MethodPost, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}
		auth, err := resolver(req)
		if err != nil {
			return nil, err
		}
		return fn(req.Context(), body, auth)
	}, mw...)
}

func routePutAuth[Req, Res, Auth any](
	r RouteBuilder,
	path string,
	fn func(context.Context, Req, Auth) (Res, error),
	mw ...Middleware,
) {
	resolver := getResolver[Auth](r)
	Handle(r, http.MethodPut, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}
		auth, err := resolver(req)
		if err != nil {
			return nil, err
		}
		return fn(req.Context(), body, auth)
	}, mw...)
}

func routePatchAuth[Req, Res, Auth any](
	r RouteBuilder,
	path string,
	fn func(context.Context, Req, Auth) (Res, error),
	mw ...Middleware,
) {
	resolver := getResolver[Auth](r)
	Handle(r, http.MethodPatch, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}
		auth, err := resolver(req)
		if err != nil {
			return nil, err
		}
		return fn(req.Context(), body, auth)
	}, mw...)
}

func routeDeleteAuth[Res, Auth any](
	r RouteBuilder,
	path string,
	fn func(context.Context, Auth) (Res, error),
	mw ...Middleware,
) {
	resolver := getResolver[Auth](r)
	Handle(r, http.MethodDelete, path, func(req *Request) (any, error) {
		auth, err := resolver(req)
		if err != nil {
			return nil, err
		}
		return fn(req.Context(), auth)
	}, mw...)
}

func routeDeleteWithAuth[Req, Res, Auth any](
	r RouteBuilder,
	path string,
	fn func(context.Context, Req, Auth) (Res, error),
	mw ...Middleware,
) {
	resolver := getResolver[Auth](r)
	Handle(r, http.MethodDelete, path, func(req *Request) (any, error) {
		body, err := bindAndValidate[Req](req)
		if err != nil {
			return nil, err
		}
		auth, err := resolver(req)
		if err != nil {
			return nil, err
		}
		return fn(req.Context(), body, auth)
	}, mw...)
}
