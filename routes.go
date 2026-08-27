// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"net/http"
	"reflect"
	"slices"
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
	s.registerRouteWithType(method, path, handler, nil, mw...)
}

func (s *Server) registerRouteWithType(method, path string, handler RawHandler, ht reflect.Type, mw ...Middleware) {
	h := handler
	for _, m := range slices.Backward(mw) {
		h = m(h)
	}

	s.router.Add(method, path, h, ht)
}

// MountRaw registers a low-level [RawHandler] on the specified HTTP method and route pattern.
func (s *Server) MountRaw(method, pattern string, handler RawHandler, mw ...Middleware) {
	s.registerRoute(method, pattern, handler, mw...)
}

// Post registers a route handler on POST: accepts any valid handler signature.
func (s *Server) Post(path string, handler any, mw ...Middleware) {
	routeUniversal(s, http.MethodPost, path, handler, mw...)
}

// Get registers a route handler on GET: accepts any valid handler signature.
func (s *Server) Get(path string, handler any, mw ...Middleware) {
	routeUniversal(s, http.MethodGet, path, handler, mw...)
}

// Put registers a route handler on PUT: accepts any valid handler signature.
func (s *Server) Put(path string, handler any, mw ...Middleware) {
	routeUniversal(s, http.MethodPut, path, handler, mw...)
}

// Patch registers a route handler on PATCH: accepts any valid handler signature.
func (s *Server) Patch(path string, handler any, mw ...Middleware) {
	routeUniversal(s, http.MethodPatch, path, handler, mw...)
}

// Delete registers a route handler on DELETE: accepts any valid handler signature.
func (s *Server) Delete(path string, handler any, mw ...Middleware) {
	routeUniversal(s, http.MethodDelete, path, handler, mw...)
}

// Options registers a route handler on OPTIONS.
func (s *Server) Options(path string, handler any, mw ...Middleware) {
	routeUniversal(s, http.MethodOptions, path, handler, mw...)
}

// Head registers a route handler on HEAD.
func (s *Server) Head(path string, handler any, mw ...Middleware) {
	routeUniversal(s, http.MethodHead, path, handler, mw...)
}
