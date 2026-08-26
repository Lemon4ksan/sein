// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
)

// Group creates a new scoped router group anchored to this server.
func (s *Server) Group(prefix string, mw ...Middleware) *Group {
	return NewGroup(s, prefix, mw...)
}

// Mount attaches a domain Module under the specified prefix with optional group middlewares.
func (s *Server) Mount(prefix string, m Module, mw ...Middleware) *Server {
	g := s.Group(prefix, mw...)
	m.Mount(g)
	return s
}

// Guard creates a protected GuardScope on the server with the specified middlewares applied.
func (s *Server) Guard(mw ...Middleware) *GuardScope {
	return &GuardScope{
		Group: s.Group("", mw...),
	}
}

// MountModule attaches a domain Module directly at root level.
func (s *Server) MountModule(m Module) *Server {
	m.Mount(s.Group(""))
	return s
}

func (s *Server) registerRoute(method, path string, handler RawHandler, mw ...Middleware) {
	Handle(s, method, path, handler, mw...)
}

// MountRaw registers a low-level RawHandler on the specified HTTP method and route pattern.
func (s *Server) MountRaw(method, pattern string, handler RawHandler, mw ...Middleware) {
	s.registerRoute(method, pattern, handler, mw...)
}

// Post registers a pure POST handler on the server: (ctx, Req) -> (Res, error)
func (s *Server) Post[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePost(s, path, fn, mw...)
}

// PostAction registers a pure parameterless POST handler on the server: (ctx) -> (Res, error)
func (s *Server) PostAction[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routePostAction(s, path, fn, mw...)
}

// PostWith is an alias for Post on the server: (ctx, Req) -> (Res, error)
func (s *Server) PostWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePost(s, path, fn, mw...)
}

// PostReq registers a POST handler with Request metadata on the server: (req, Req) -> (Res, error)
func (s *Server) PostReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	routePostReq(s, path, fn, mw...)
}

// Get registers a pure GET handler on the server: (ctx) -> (Res, error)
func (s *Server) Get[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routeGet(s, path, fn, mw...)
}

// GetWith registers a pure GET handler with Path/Query/Header DTO on the server: (ctx, Req) -> (Res, error)
func (s *Server) GetWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routeGetWith(s, path, fn, mw...)
}

// GetReq registers a GET handler with Request metadata on the server: (req) -> (Res, error)
func (s *Server) GetReq[Res any](path string, fn func(*Request) (Res, error), mw ...Middleware) {
	routeGetReq(s, path, fn, mw...)
}

// Put registers a pure PUT handler on the server: (ctx, Req) -> (Res, error)
func (s *Server) Put[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePut(s, path, fn, mw...)
}

// PutAction registers a pure parameterless PUT handler on the server: (ctx) -> (Res, error)
func (s *Server) PutAction[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routePutAction(s, path, fn, mw...)
}

// PutWith is an alias for Put on the server: (ctx, Req) -> (Res, error)
func (s *Server) PutWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePut(s, path, fn, mw...)
}

// PutReq registers a PUT handler with Request metadata on the server: (req, Req) -> (Res, error)
func (s *Server) PutReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	routePutReq(s, path, fn, mw...)
}

// Patch registers a pure PATCH handler on the server: (ctx, Req) -> (Res, error)
func (s *Server) Patch[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePatch(s, path, fn, mw...)
}

// PatchAction registers a pure parameterless PATCH handler on the server: (ctx) -> (Res, error)
func (s *Server) PatchAction[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routePatchAction(s, path, fn, mw...)
}

// PatchReq registers a PATCH handler with Request metadata on the server: (req, Req) -> (Res, error)
func (s *Server) PatchReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	routePatchReq(s, path, fn, mw...)
}

// Delete registers a pure DELETE handler on the server: (ctx) -> (Res, error)
func (s *Server) Delete[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routeDelete(s, path, fn, mw...)
}

// DeleteWith registers a pure DELETE handler with Path/Query DTO on the server: (ctx, Req) -> (Res, error)
func (s *Server) DeleteWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routeDeleteWith(s, path, fn, mw...)
}

// DeleteReq registers a DELETE handler with Request metadata on the server: (req) -> (Res, error)
func (s *Server) DeleteReq[Res any](path string, fn func(*Request) (Res, error), mw ...Middleware) {
	routeDeleteReq(s, path, fn, mw...)
}
