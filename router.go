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
	paramPrefix   string
	paramName     string
	handler       RawHandler
	children      []*routeNode
}

// Router represents a high-throughput hybrid HTTP routing engine combining
// an O(1) hash-indexed static lookup table with a compact Radix Trie for parameterized routes.
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

	var segBuf [16]string
	segments := splitPathBuf(pattern, &segBuf)
	hasTrailingSlash := len(pattern) > 1 && strings.HasSuffix(pattern, "/")

	insertNode(root, segments, handler, hasTrailingSlash)
}

func insertNode(curr *routeNode, segments []string, handler RawHandler, hasTrailingSlash bool) {
	if len(segments) == 0 {
		curr.handler = handler
		curr.trailingSlash = hasTrailingSlash

		return
	}

	seg := segments[0]
	remaining := segments[1:]

	isWildcard := strings.HasPrefix(seg, "*") || strings.HasPrefix(seg, "...")
	isParam := false
	paramPrefix := ""
	paramName := ""

	if isWildcard {
		if strings.HasPrefix(seg, "*") {
			paramName = strings.TrimPrefix(seg, "*")
		} else {
			paramName = strings.TrimPrefix(seg, "...")
		}
	} else if idx := strings.IndexByte(seg, ':'); idx != -1 {
		isParam = true
		paramPrefix = seg[:idx]
		paramName = seg[idx+1:]
	}

	// Look for existing matching child
	for _, child := range curr.children {
		if child.isWildcard == isWildcard && child.isParam == isParam && child.pathSegment == seg {
			insertNode(child, remaining, handler, hasTrailingSlash)
			return
		}
	}

	newNode := &routeNode{
		pathSegment: seg,
		isParam:     isParam,
		isWildcard:  isWildcard,
		paramPrefix: paramPrefix,
		paramName:   paramName,
	}

	curr.children = append(curr.children, newNode)
	insertNode(newNode, remaining, handler, hasTrailingSlash)
}

// Match searches the routing tree for a registered [RawHandler] matching the HTTP method and path.
// When matched, extracted path variables are populated into params without heap allocations.
func (r *Router) Match(method, path string, params *Params) (RawHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Fast Path: Check static route table (0 B/op, O(1) lookup)
	if m, ok := r.static[method]; ok {
		if h, ok := m[path]; ok {
			return h, true
		}
	}

	root, ok := r.routes[method]
	if !ok {
		return nil, false
	}

	var segBuf [16]string
	segments := splitPathBuf(path, &segBuf)
	hasTrailingSlash := len(path) > 1 && strings.HasSuffix(path, "/")

	handler, ok := matchNode(root, segments, hasTrailingSlash, params)
	if !ok {
		return nil, false
	}

	return handler, true
}

func matchNode(curr *routeNode, segments []string, hasTrailingSlash bool, params *Params) (RawHandler, bool) {
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
			if child.paramPrefix != "" && !strings.HasPrefix(seg, child.paramPrefix) {
				continue
			}

			if params != nil {
				paramVal := seg
				if child.paramPrefix != "" {
					paramVal = strings.TrimPrefix(seg, child.paramPrefix)
				}
				params.Set(child.paramName, paramVal)
			}
			if h, ok := matchNode(child, remaining, hasTrailingSlash, params); ok {
				return h, true
			}
		}
	}

	// 3. Try wildcard match
	for _, child := range curr.children {
		if child.isWildcard {
			if child.paramName != "" && params != nil {
				params.Set(child.paramName, strings.Join(segments, "/"))
			}

			if child.handler != nil {
				return child.handler, true
			}
		}
	}

	return nil, false
}

func splitPathBuf(path string, buf *[16]string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}

	count := 0
	start := 0
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] == '/' {
			if count < len(buf) {
				buf[count] = trimmed[start:i]
				count++
			}
			start = i + 1
		}
	}
	if start <= len(trimmed) && count < len(buf) {
		buf[count] = trimmed[start:]
		count++
	}

	if count <= len(buf) {
		return buf[:count]
	}

	return strings.Split(trimmed, "/")
}

// HasPath reports whether the specified URL path matches any registered route under any HTTP verb.
func (r *Router) HasPath(path string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, m := range r.static {
		if _, ok := m[path]; ok {
			return true
		}
	}

	var segBuf [16]string
	segments := splitPathBuf(path, &segBuf)
	hasTrailingSlash := len(path) > 1 && strings.HasSuffix(path, "/")
	for _, root := range r.routes {
		if _, ok := matchNode(root, segments, hasTrailingSlash, nil); ok {
			return true
		}
	}

	return false
}

// AllowedMethods returns all uppercase HTTP methods registered for the specified URL path.
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
	var segBuf [16]string
	segments := splitPathBuf(path, &segBuf)
	hasTrailingSlash := len(path) > 1 && strings.HasSuffix(path, "/")
	for m, root := range r.routes {
		if slices.Contains(methods, m) {
			continue
		}

		if _, ok := matchNode(root, segments, hasTrailingSlash, nil); ok {
			methods = append(methods, m)
		}
	}

	slices.Sort(methods)

	return methods
}

// FindTrailingSlash tests if inverting the trailing slash state of path matches a registered route.
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

	if _, ok := r.Match(method, altPath, nil); ok {
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
