// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
	"slices"
	"strings"
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
	parent      RouteBuilder
	prefix      string
	middlewares []Middleware
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

// Group creates a nested subgroup inheriting the current prefix and middlewares.
func (g *Group) Group(prefix string, mw ...Middleware) *Group {
	combinedPrefix := joinPaths(g.prefix, prefix)
	combinedMW := append(slices.Clone(g.middlewares), mw...)

	return &Group{
		parent:      g.parent,
		prefix:      combinedPrefix,
		middlewares: combinedMW,
	}
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

// Use appends middlewares to the group.
func (g *Group) Use(mw ...Middleware) {
	g.middlewares = append(g.middlewares, mw...)
}

func (g *Group) registerRoute(method, path string, handler RawHandler, mw ...Middleware) {
	fullPath := joinPaths(g.prefix, path)
	combinedMW := append(slices.Clone(g.middlewares), mw...)
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

	if path == "" || path == "/" {
		return base
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return strings.TrimSuffix(base, "/") + path
}
