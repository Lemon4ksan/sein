// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
)

// Module represents a self-contained domain component that mounts its endpoints onto a Group.
type Module interface {
	Mount(g *Group)
}

// ModuleFunc is a functional adapter that satisfies the Module interface.
type ModuleFunc func(g *Group)

// Mount satisfies the Module interface for ModuleFunc.
func (f ModuleFunc) Mount(g *Group) {
	f(g)
}

// RouteBuilder is the route-registration interface implemented by both *Server and *Group.
type RouteBuilder interface {
	registerRoute(method, path string, handler RawHandler, mw ...Middleware)
}

// Group is a scoped sub-router with a common URL prefix and middleware chain.
type Group struct {
	parent       RouteBuilder
	prefix       string
	middlewares  []Middleware
	errorMappers []ErrorMapper
	resolvers    sync.Map
}

// NewGroup creates a new Group anchored to a parent RouteBuilder.
func NewGroup(parent RouteBuilder, prefix string, mw ...Middleware) *Group {
	return &Group{
		parent:      parent,
		prefix:      cleanPrefix(prefix),
		middlewares: mw,
	}
}

// Mount attaches a nested domain Module under the specified prefix with optional group middlewares.
func (g *Group) Mount(prefix string, m Module, mw ...Middleware) *Group {
	sub := g.Group(prefix, mw...)
	m.Mount(sub)
	return g
}

// Group creates a nested subgroup inheriting the current prefix, middlewares, and error mappers.
func (g *Group) Group(prefix string, mw ...Middleware) *Group {
	combinedPrefix := joinPaths(g.prefix, prefix)
	combinedMW := append(slices.Clone(g.middlewares), mw...)
	combinedMappers := slices.Clone(g.errorMappers)

	sub := &Group{
		parent:       g.parent,
		prefix:       combinedPrefix,
		middlewares:  combinedMW,
		errorMappers: combinedMappers,
	}

	g.resolvers.Range(func(key, value any) bool {
		sub.resolvers.Store(key, value)
		return true
	})

	return sub
}

// MapError registers a scoped mapping from a sentinel error target to a DomainError for all routes in this group.
func (g *Group) MapError(target error, domainErr DomainError) *Group {
	g.errorMappers = append(g.errorMappers, func(err error) (DomainError, bool) {
		if errors.Is(err, target) {
			return domainErr, true
		}

		return nil, false
	})

	return g
}

// MapErrorFunc registers a custom scoped error mapping predicate for all routes in this group.
func (g *Group) MapErrorFunc(fn ErrorMapper) *Group {
	g.errorMappers = append(g.errorMappers, fn)
	return g
}

// Guard creates a protected GuardScope inheriting the group's prefix with the specified middlewares applied.
func (g *Group) Guard(mw ...Middleware) *GuardScope {
	return &GuardScope{
		Group: g.Group("", mw...),
	}
}

// GuardScope encapsulates a scoped collection of routes and modules protected by a shared middleware guard.
// It embeds *Group, allowing all standard HTTP routing and module mounting methods to be called directly.
type GuardScope struct {
	*Group
}

// Do executes a configuration callback function receiving the protected *Group.
func (gs *GuardScope) Do(fn func(g *Group)) *GuardScope {
	fn(gs.Group)
	return gs
}

// Mount mounts a domain Module under prefix with the guard's middlewares applied.
func (gs *GuardScope) Mount(prefix string, m Module, extraMW ...Middleware) *GuardScope {
	gs.Group.Mount(prefix, m, extraMW...)
	return gs
}

// MountModule mounts a domain Module directly at the current level with the guard's middlewares applied.
func (gs *GuardScope) MountModule(m Module) *GuardScope {
	m.Mount(gs.Group)
	return gs
}

// MapError registers a scoped error mapping on the guard scope.
func (gs *GuardScope) MapError(target error, domainErr DomainError) *GuardScope {
	gs.Group.MapError(target, domainErr)
	return gs
}

// Use appends middlewares to the group.
func (g *Group) Use(mw ...Middleware) {
	g.middlewares = append(g.middlewares, mw...)
}

func (g *Group) registerRoute(method, path string, handler RawHandler, mw ...Middleware) {
	fullPath := joinPaths(g.prefix, path)
	combinedMW := append(slices.Clone(g.middlewares), mw...)

	if len(g.errorMappers) > 0 {
		mappers := slices.Clone(g.errorMappers)
		wrappedHandler := func(req *Request) (any, error) {
			res, err := handler(req)
			if err != nil {
				for _, mapper := range mappers {
					if mapped, ok := mapper(err); ok {
						return nil, mapped
					}
				}
				return nil, err
			}
			return res, nil
		}
		g.parent.registerRoute(method, fullPath, wrappedHandler, combinedMW...)
		return
	}

	g.parent.registerRoute(method, fullPath, handler, combinedMW...)
}

// Post registers a pure POST handler on this group: (ctx, Req) -> (Res, error)
func (g *Group) Post[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePost(g, path, fn, mw...)
}

// PostAction registers a pure parameterless POST handler on this group: (ctx) -> (Res, error)
func (g *Group) PostAction[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routePostAction(g, path, fn, mw...)
}

// PostWith is an alias for Post on this group: (ctx, Req) -> (Res, error)
func (g *Group) PostWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePost(g, path, fn, mw...)
}

// PostReq registers a POST handler with Request metadata on this group: (req, Req) -> (Res, error)
func (g *Group) PostReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	routePostReq(g, path, fn, mw...)
}

// Get registers a pure GET handler on this group: (ctx) -> (Res, error)
func (g *Group) Get[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routeGet(g, path, fn, mw...)
}

// GetWith registers a pure GET handler with Path/Query/Header DTO on this group: (ctx, Req) -> (Res, error)
func (g *Group) GetWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routeGetWith(g, path, fn, mw...)
}

// GetReq registers a GET handler with Request metadata on this group: (req) -> (Res, error)
func (g *Group) GetReq[Res any](path string, fn func(*Request) (Res, error), mw ...Middleware) {
	routeGetReq(g, path, fn, mw...)
}

// Put registers a pure PUT handler on this group: (ctx, Req) -> (Res, error)
func (g *Group) Put[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePut(g, path, fn, mw...)
}

// PutWith is an alias for Put on this group: (ctx, Req) -> (Res, error)
func (g *Group) PutWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePut(g, path, fn, mw...)
}

// PutReq registers a PUT handler with Request metadata on this group: (req, Req) -> (Res, error)
func (g *Group) PutReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	routePutReq(g, path, fn, mw...)
}

// Patch registers a pure PATCH handler on this group: (ctx, Req) -> (Res, error)
func (g *Group) Patch[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePatch(g, path, fn, mw...)
}

// PatchWith is an alias for Patch on this group: (ctx, Req) -> (Res, error)
func (g *Group) PatchWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePatch(g, path, fn, mw...)
}

// PatchReq registers a PATCH handler with Request metadata on this group: (req, Req) -> (Res, error)
func (g *Group) PatchReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	routePatchReq(g, path, fn, mw...)
}

// Delete registers a pure DELETE handler on this group: (ctx) -> (Res, error)
func (g *Group) Delete[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routeDelete(g, path, fn, mw...)
}

// DeleteWith registers a pure DELETE handler with Path/Query DTO on this group: (ctx, Req) -> (Res, error)
func (g *Group) DeleteWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routeDeleteWith(g, path, fn, mw...)
}

// DeleteReq registers a DELETE handler with Request metadata on this group: (req) -> (Res, error)
func (g *Group) DeleteReq[Res any](path string, fn func(*Request) (Res, error), mw ...Middleware) {
	routeDeleteReq(g, path, fn, mw...)
}

func cleanPrefix(prefix string) string {
	if prefix == "" || prefix == "/" {
		return ""
	}

	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}

	return strings.TrimSuffix(prefix, "/")
}

func joinPaths(base, path string) string {
	if base == "" {
		if path == "" {
			return "/"
		}

		if !strings.HasPrefix(path, "/") {
			return "/" + path
		}

		return path
	}

	if path == "" {
		return base
	}

	if path == "/" {
		return strings.TrimSuffix(base, "/") + "/"
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return strings.TrimSuffix(base, "/") + path
}
