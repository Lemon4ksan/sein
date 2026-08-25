// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
	"slices"
	"strings"
)

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

// Use appends middlewares to the group.
func (g *Group) Use(mw ...Middleware) {
	g.middlewares = append(g.middlewares, mw...)
}

func (g *Group) registerRoute(method, path string, handler RawHandler, mw ...Middleware) {
	fullPath := joinPaths(g.prefix, path)
	combinedMW := append(slices.Clone(g.middlewares), mw...)
	g.parent.registerRoute(method, fullPath, handler, combinedMW...)
}

// POST registers a pure POST handler on this group: (ctx, Req) -> (Res, error)
func (g *Group) POST[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	POST(g, path, fn, mw...)
}

// POSTReq registers a POST handler with Request metadata on this group: (req, Req) -> (Res, error)
func (g *Group) POSTReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	POSTReq(g, path, fn, mw...)
}

// GET registers a pure GET handler on this group: (ctx) -> (Res, error)
func (g *Group) GET[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	GET(g, path, fn, mw...)
}

// GETReq registers a GET handler with Request metadata on this group: (req) -> (Res, error)
func (g *Group) GETReq[Res any](path string, fn func(*Request) (Res, error), mw ...Middleware) {
	GETReq(g, path, fn, mw...)
}

// PUT registers a pure PUT handler on this group: (ctx, Req) -> (Res, error)
func (g *Group) PUT[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	PUT(g, path, fn, mw...)
}

// PUTReq registers a PUT handler with Request metadata on this group: (req, Req) -> (Res, error)
func (g *Group) PUTReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	PUTReq(g, path, fn, mw...)
}

// PATCH registers a pure PATCH handler on this group: (ctx, Req) -> (Res, error)
func (g *Group) PATCH[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	PATCH(g, path, fn, mw...)
}

// PATCHReq registers a PATCH handler with Request metadata on this group: (req, Req) -> (Res, error)
func (g *Group) PATCHReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	PATCHReq(g, path, fn, mw...)
}

// DELETE registers a pure DELETE handler on this group: (ctx) -> (Res, error)
func (g *Group) DELETE[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	DELETE(g, path, fn, mw...)
}

// DELETEReq registers a DELETE handler with Request metadata on this group: (req) -> (Res, error)
func (g *Group) DELETEReq[Res any](path string, fn func(*Request) (Res, error), mw ...Middleware) {
	DELETEReq(g, path, fn, mw...)
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
