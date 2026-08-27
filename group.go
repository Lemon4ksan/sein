// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"errors"
	"net/http"
	"reflect"
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

// Mount implements Module for ModuleFunc.
func (f ModuleFunc) Mount(g *Group) {
	f(g)
}

// RouteBuilder is the common abstraction shared between Server and Group.
type RouteBuilder interface {
	registerRoute(method, path string, handler RawHandler, mw ...Middleware)
	registerRouteWithType(method, path string, handler RawHandler, ht reflect.Type, mw ...Middleware)
}

// ErrorMapperFunc translates internal sentinel errors into typed DomainErrors.
type ErrorMapperFunc func(error) (DomainError, bool)

// Group represents a scoped router group with a path prefix, scoped middlewares, and domain error mappers.
type Group struct {
	parent       RouteBuilder
	prefix       string
	middlewares  []Middleware
	errorMappers []ErrorMapperFunc
	resolvers    sync.Map
	mu           sync.RWMutex
}

// NewGroup creates a new route Group attached to a parent RouteBuilder.
func NewGroup(parent RouteBuilder, prefix string, mw ...Middleware) *Group {
	return &Group{
		parent:      parent,
		prefix:      cleanPrefix(prefix),
		middlewares: slices.Clone(mw),
	}
}

// Group creates a nested sub-group under this group's prefix.
func (g *Group) Group(prefix string, mw ...Middleware) *Group {
	fullPrefix := joinPaths(g.prefix, prefix)
	sub := &Group{
		parent:       g.parent,
		prefix:       cleanPrefix(fullPrefix),
		middlewares:  append(slices.Clone(g.middlewares), mw...),
		errorMappers: slices.Clone(g.errorMappers),
	}

	return sub
}

// Mount attaches a domain Module under this group with optional additional middlewares.
func (g *Group) Mount(prefix string, m Module, mw ...Middleware) *Group {
	sub := g.Group(prefix, mw...)
	m.Mount(sub)
	return g
}

// Guard creates a protected GuardScope within this group with the given middlewares applied.
func (g *Group) Guard(mw ...Middleware) *GuardScope {
	return &GuardScope{
		Group: g.Group("", mw...),
	}
}

// GuardScope represents a protected route scope that can conditionally mount routes via Do().
type GuardScope struct {
	*Group
}

// Do executes the callback within the protected GuardScope.
func (gs *GuardScope) Do(fn func(g *Group)) *GuardScope {
	fn(gs.Group)
	return gs
}

// MapError registers a domain error mapping rule on the guard scope.
func (gs *GuardScope) MapError(target error, domainErr DomainError) *GuardScope {
	gs.Group.MapError(target, domainErr)
	return gs
}

// MapErrors registers multiple scoped error mappings from an Errors table on the guard scope.
func (gs *GuardScope) MapErrors(errorsMap Errors) *GuardScope {
	gs.Group.MapErrors(errorsMap)
	return gs
}

// Use appends middlewares to the group.
func (g *Group) Use(mw ...Middleware) {
	g.middlewares = append(g.middlewares, mw...)
}

func (g *Group) registerRoute(method, path string, handler RawHandler, mw ...Middleware) {
	g.registerRouteWithType(method, path, handler, nil, mw...)
}

func (g *Group) registerRouteWithType(method, path string, handler RawHandler, ht reflect.Type, mw ...Middleware) {
	fullPath := joinPaths(g.prefix, path)
	combinedMW := append(slices.Clone(g.middlewares), mw...)

	if len(g.errorMappers) == 0 {
		g.parent.registerRouteWithType(method, fullPath, handler, ht, combinedMW...)
		return
	}

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
	g.parent.registerRouteWithType(method, fullPath, wrappedHandler, ht, combinedMW...)
}

// MapError registers a mapping from an internal sentinel error to a Sein domain error.
func (g *Group) MapError(target error, domainErr DomainError) *Group {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.errorMappers = append(g.errorMappers, func(err error) (DomainError, bool) {
		if errors.Is(err, target) {
			return domainErr, true
		}
		return nil, false
	})

	return g
}

// MapErrors registers multiple error mappings on the group using an Errors table.
func (g *Group) MapErrors(errorsMap Errors) *Group {
	g.mu.Lock()
	defer g.mu.Unlock()

	for target, domainErr := range errorsMap {
		t := target
		d := domainErr
		g.errorMappers = append(g.errorMappers, func(err error) (DomainError, bool) {
			if errors.Is(err, t) {
				return d, true
			}
			return nil, false
		})
	}

	return g
}

// Post registers a route handler on POST on this group: accepts any valid handler signature.
func (g *Group) Post(path string, handler any, mw ...Middleware) {
	routeUniversal(g, http.MethodPost, path, handler, mw...)
}

// Get registers a route handler on GET on this group: accepts any valid handler signature.
func (g *Group) Get(path string, handler any, mw ...Middleware) {
	routeUniversal(g, http.MethodGet, path, handler, mw...)
}

// Put registers a route handler on PUT on this group: accepts any valid handler signature.
func (g *Group) Put(path string, handler any, mw ...Middleware) {
	routeUniversal(g, http.MethodPut, path, handler, mw...)
}

// Patch registers a route handler on PATCH on this group: accepts any valid handler signature.
func (g *Group) Patch(path string, handler any, mw ...Middleware) {
	routeUniversal(g, http.MethodPatch, path, handler, mw...)
}

// Delete registers a route handler on DELETE on this group: accepts any valid handler signature.
func (g *Group) Delete(path string, handler any, mw ...Middleware) {
	routeUniversal(g, http.MethodDelete, path, handler, mw...)
}

// Options registers a route handler on OPTIONS on this group.
func (g *Group) Options(path string, handler any, mw ...Middleware) {
	routeUniversal(g, http.MethodOptions, path, handler, mw...)
}

// Head registers a route handler on HEAD on this group.
func (g *Group) Head(path string, handler any, mw ...Middleware) {
	routeUniversal(g, http.MethodHead, path, handler, mw...)
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
