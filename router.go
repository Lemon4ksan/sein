// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"slices"
	"strings"
	"sync"
)

// RouteInfo encapsulates metadata describing a registered route in the server routing tree.
// It is used for route inspection, introspection APIs, and automated OpenAPI schema generation.
type RouteInfo struct {
	// Method is the uppercase HTTP verb (e.g., "GET", "POST", "PUT", "DELETE").
	Method string

	// Path is the URL route pattern (e.g., "/users/:id", "/assets/*filepath").
	Path string
}

type routeNode struct {
	pathSegment   string
	isParam       bool
	isWildcard    bool
	trailingSlash bool
	paramName     string
	handler       RawHandler
	children      []*routeNode
}

// Router represents a high-throughput hybrid HTTP routing engine combining
// an O(1) hash-indexed static lookup table with a compact Radix Trie for parameterized routes.
//
// # Architectural Philosophy: Dual-Path Resolution
//
// To achieve maximum throughput with 0 heap allocations per request (0 B/op), the router
// employs a two-tier resolution strategy:
//
//  1. Fast Path: Static routes (routes without ":" parameters or "*" wildcards) are stored in
//     a direct hash map and resolved in O(1) time without path splitting or node traversal.
//  2. Trie Fallback: Dynamic and wildcard routes are resolved using a compact Radix Trie
//     where path segments are mapped into parameter captures.
//
// # Concurrency & Thread-Safety Invariants
//
// Router is thread-safe for concurrent reads and writes across multiple goroutines,
// guarded by an internal [sync.RWMutex].
type Router struct {
	mu        sync.RWMutex
	routes    map[string]*routeNode
	static    map[string]map[string]RawHandler
	routeList []RouteInfo
}

// NewRouter instantiates an empty, initialized [Router] ready for route registrations.
func NewRouter() *Router {
	return &Router{
		routes:    make(map[string]*routeNode),
		static:    make(map[string]map[string]RawHandler),
		routeList: make([]RouteInfo, 0, 32),
	}
}

// Add registers a new HTTP route pattern and its associated [RawHandler].
//
// Route Patterns:
//   - Static: "/api/v1/health" (O(1) direct hash lookup)
//   - Parameterized: "/users/:id" (captures "id" into request parameters)
//   - Catch-all Wildcard: "/static/*filepath" or "/assets/..." (captures remaining path)
//
// Usage:
//
//	router.Add("GET", "/users/:id", handler)
//	router.Add("POST", "/api/v1/checkout", handler)
//
// Parameters:
//   - method: Uppercase HTTP verb string (e.g., "GET", "POST", "PUT").
//   - pattern: URL pattern supporting exact segments, ":param" placeholders, and "*param" wildcards.
//   - handler: The [RawHandler] function invoked when a matching request arrives.
func (r *Router) Add(method, pattern string, handler RawHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.routeList = append(r.routeList, RouteInfo{
		Method: method,
		Path:   pattern,
	})

	// 1. If pattern has no dynamic parameters, register into O(1) static lookup table
	if !strings.Contains(pattern, ":") && !strings.Contains(pattern, "*") && !strings.Contains(pattern, "...") {
		if r.static[method] == nil {
			r.static[method] = make(map[string]RawHandler)
		}
		r.static[method][pattern] = handler
	}

	// 2. Also register into Trie for fallback and traversal
	root, ok := r.routes[method]
	if !ok {
		root = &routeNode{}
		r.routes[method] = root
	}

	segments := splitPath(pattern)
	curr := root

	for _, seg := range segments {
		if seg == "" {
			continue
		}

		isWildcard := strings.HasPrefix(seg, "*") || seg == "..."
		isParam := !isWildcard && strings.HasPrefix(seg, ":")
		paramName := ""
		if isParam {
			paramName = seg[1:]
		} else if isWildcard {
			if strings.HasPrefix(seg, "*") {
				paramName = seg[1:]
			}
		}

		var child *routeNode
		for _, c := range curr.children {
			if isWildcard && c.isWildcard && c.paramName == paramName {
				child = c
				break
			}
			if isParam && c.isParam && c.paramName == paramName {
				child = c
				break
			}
			if !isParam && !isWildcard && !c.isParam && !c.isWildcard && c.pathSegment == seg {
				child = c
				break
			}
		}

		if child == nil {
			child = &routeNode{
				pathSegment: seg,
				isParam:     isParam,
				isWildcard:  isWildcard,
				paramName:   paramName,
			}
			curr.children = append(curr.children, child)
		}

		curr = child
	}

	curr.handler = handler
	curr.trailingSlash = len(pattern) > 1 && strings.HasSuffix(pattern, "/")
}

// Match resolves the incoming HTTP method and request path against registered route trees.
//
// Performance & Complexity:
//   - Static Routes: O(1) direct hash-table lookup with 0 heap allocations.
//   - Dynamic Routes: O(K) where K is the number of path segments.
//
// Returns:
//   - handler: The matched [RawHandler], or nil if no match exists.
//   - params: Extracted URL path parameters (e.g. {"id": "42"}), or nil for static routes.
//   - found: True if a matching route was found; false otherwise.
func (r *Router) Match(method, path string) (RawHandler, map[string]string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Fast Path: Check static route table (0 B/op, O(1) lookup)
	if m, ok := r.static[method]; ok {
		if h, ok := m[path]; ok {
			return h, nil, true
		}
	}

	root, ok := r.routes[method]
	if !ok {
		return nil, nil, false
	}

	segments := splitPath(path)
	params := make(map[string]string)
	hasTrailingSlash := len(path) > 1 && strings.HasSuffix(path, "/")

	handler, ok := matchNode(root, segments, hasTrailingSlash, params)
	if !ok {
		return nil, nil, false
	}

	return handler, params, true
}

func matchNode(curr *routeNode, segments []string, hasTrailingSlash bool, params map[string]string) (RawHandler, bool) {
	if len(segments) == 0 {
		if curr.handler != nil {
			if curr.isWildcard || curr.trailingSlash == hasTrailingSlash {
				return curr.handler, true
			}
			return nil, false
		}
		// Check for wildcard matching empty suffix
		for _, child := range curr.children {
			if child.isWildcard && child.handler != nil {
				return child.handler, true
			}
		}
		return nil, false
	}

	seg := segments[0]
	remaining := segments[1:]

	// 1. Try exact match first
	for _, child := range curr.children {
		if !child.isParam && !child.isWildcard && child.pathSegment == seg {
			if h, ok := matchNode(child, remaining, hasTrailingSlash, params); ok {
				return h, true
			}
		}
	}

	// 2. Try param match
	for _, child := range curr.children {
		if child.isParam {
			params[child.paramName] = seg
			if h, ok := matchNode(child, remaining, hasTrailingSlash, params); ok {
				return h, true
			}
			delete(params, child.paramName)
		}
	}

	// 3. Try wildcard match
	for _, child := range curr.children {
		if child.isWildcard {
			if child.paramName != "" {
				params[child.paramName] = strings.Join(segments, "/")
			}
			if child.handler != nil {
				return child.handler, true
			}
		}
	}

	return nil, false
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// HasPath reports whether the specified URL path matches any registered route under any HTTP verb.
// It is used for CORS OPTIONS preflight detection and Method Not Allowed evaluations.
func (r *Router) HasPath(path string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, m := range r.static {
		if _, ok := m[path]; ok {
			return true
		}
	}

	segments := splitPath(path)
	params := make(map[string]string)
	hasTrailingSlash := len(path) > 1 && strings.HasSuffix(path, "/")
	for _, root := range r.routes {
		if _, ok := matchNode(root, segments, hasTrailingSlash, params); ok {
			return true
		}
	}

	return false
}

// AllowedMethods returns all uppercase HTTP methods registered for the specified URL path.
// The returned slice is sorted in ascending alphabetical order (e.g. ["GET", "POST", "PUT"]).
//
// Usage:
//
//	methods := router.AllowedMethods("/api/v1/users")
//	// methods -> ["GET", "POST"]
func (r *Router) AllowedMethods(path string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var methods []string

	// Check static routes
	for m, routes := range r.static {
		if _, ok := routes[path]; ok {
			methods = append(methods, m)
		}
	}

	// Check dynamic trie routes
	segments := splitPath(path)
	params := make(map[string]string)
	hasTrailingSlash := len(path) > 1 && strings.HasSuffix(path, "/")
	for m, root := range r.routes {
		if slices.Contains(methods, m) {
			continue
		}
		if _, ok := matchNode(root, segments, hasTrailingSlash, params); ok {
			methods = append(methods, m)
		}
	}

	slices.Sort(methods)
	return methods
}

// FindTrailingSlash tests if inverting the trailing slash state of path matches a registered route.
//
// If path is "/users/" and "/users" exists, it returns ("/users", true).
// If path is "/users" and "/users/" exists, it returns ("/users/", true).
//
// Returns:
//   - canonicalPath: The corrected URL path if a valid route exists.
//   - found: True if an inverted trailing slash match exists; false otherwise.
func (r *Router) FindTrailingSlash(method, path string) (string, bool) {
	if path == "/" {
		return "", false
	}

	var altPath string
	if strings.HasSuffix(path, "/") {
		altPath = strings.TrimSuffix(path, "/")
	} else {
		altPath = path + "/"
	}

	if _, _, ok := r.Match(method, altPath); ok {
		return altPath, true
	}

	return "", false
}

// Routes returns an immutable snapshot slice of all registered routes across all HTTP methods.
func (r *Router) Routes() []RouteInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]RouteInfo, len(r.routeList))
	copy(res, r.routeList)
	return res
}
