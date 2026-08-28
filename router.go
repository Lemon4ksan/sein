// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"reflect"
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

	// HandlerType is the reflected type of the handler function for automated OpenAPI introspection.
	HandlerType reflect.Type
}

type staticRoute struct {
	handler RawHandler
	pattern string
}

type routeNode struct {
	pathSegment   string
	pattern       string
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
	static    map[string]map[string]staticRoute
	routeList []RouteInfo
}

// NewRouter instantiates an empty, initialized [Router] ready for route registrations.
func NewRouter() *Router {
	return &Router{
		routes:    make(map[string]*routeNode),
		static:    make(map[string]map[string]staticRoute),
		routeList: make([]RouteInfo, 0, 32),
	}
}

// Add registers a new HTTP route pattern and its associated [RawHandler].
func (r *Router) Add(method, pattern string, handler RawHandler, handlerType ...reflect.Type) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var ht reflect.Type
	if len(handlerType) > 0 {
		ht = handlerType[0]
	}

	r.routeList = append(r.routeList, RouteInfo{
		Method:      method,
		Path:        pattern,
		HandlerType: ht,
	})

	// 1. If pattern has no dynamic parameters, register into O(1) static lookup table
	if !strings.Contains(pattern, ":") && !strings.Contains(pattern, "*") && !strings.Contains(pattern, "...") {
		if r.static[method] == nil {
			r.static[method] = make(map[string]staticRoute)
		}

		r.static[method][pattern] = staticRoute{
			handler: handler,
			pattern: pattern,
		}
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

	insertNode(root, segments, handler, pattern, hasTrailingSlash)
}

func insertNode(curr *routeNode, segments []string, handler RawHandler, pattern string, hasTrailingSlash bool) {
	if len(segments) == 0 {
		curr.handler = handler
		curr.pattern = pattern
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
		if after, ok := strings.CutPrefix(seg, "*"); ok {
			paramName = after
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
			insertNode(child, remaining, handler, pattern, hasTrailingSlash)
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
	insertNode(newNode, remaining, handler, pattern, hasTrailingSlash)
}

// Match searches the routing tree for a registered [RawHandler] matching the HTTP method and path.
// When matched, extracted path variables are populated into params without heap allocations,
// and the matched route pattern is returned.
func (r *Router) Match(method, path string, params *Params) (RawHandler, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Fast Path: Check static route table (0 B/op, O(1) lookup)
	if m, ok := r.static[method]; ok {
		if s, ok := m[path]; ok {
			return s.handler, s.pattern, true
		}
	}

	root, ok := r.routes[method]
	if !ok {
		return nil, "", false
	}

	var segBuf [16]string
	segments := splitPathBuf(path, &segBuf)
	hasTrailingSlash := len(path) > 1 && strings.HasSuffix(path, "/")

	node, ok := matchNode(root, segments, hasTrailingSlash, params)
	if !ok || node == nil || node.handler == nil {
		return nil, "", false
	}

	return node.handler, node.pattern, true
}

func matchNode(curr *routeNode, segments []string, hasTrailingSlash bool, params *Params) (*routeNode, bool) {
	if len(segments) == 0 {
		if curr.handler != nil {
			if curr.isWildcard || curr.trailingSlash == hasTrailingSlash {
				return curr, true
			}

			return nil, false
		}

		// Check for wildcard matching empty suffix
		for _, child := range curr.children {
			if child.isWildcard && child.handler != nil {
				return child, true
			}
		}

		return nil, false
	}

	seg := segments[0]
	remaining := segments[1:]

	// 1. Try exact match first
	for _, child := range curr.children {
		if !child.isParam && !child.isWildcard && child.pathSegment == seg {
			if n, ok := matchNode(child, remaining, hasTrailingSlash, params); ok {
				return n, true
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
			if n, ok := matchNode(child, remaining, hasTrailingSlash, params); ok {
				return n, true
			}
		}
	}

	// 3. Try wildcard match
	for _, child := range curr.children {
		if child.isWildcard {
			if child.paramName != "" && params != nil {
				params.Set(child.paramName, strings.Join(segments, "/"))
			}

			return child, true
		}
	}

	return nil, false
}

// FindTrailingSlash tests if an alternate route exists with the opposite trailing slash.
func (r *Router) FindTrailingSlash(method, path string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	altPath := path + "/"
	if trimmed, ok := strings.CutSuffix(path, "/"); ok {
		altPath = trimmed
	}

	if m, ok := r.static[method]; ok {
		if _, ok := m[altPath]; ok {
			return altPath, true
		}
	}

	root, ok := r.routes[method]
	if !ok {
		return "", false
	}

	var segBuf [16]string
	segments := splitPathBuf(altPath, &segBuf)
	hasTrailingSlash := len(altPath) > 1 && strings.HasSuffix(altPath, "/")

	if n, ok := matchNode(root, segments, hasTrailingSlash, nil); ok && n != nil && n.handler != nil {
		return altPath, true
	}

	return "", false
}

// AllowedMethods returns all HTTP verbs registered for a given path across all routing trees.
func (r *Router) AllowedMethods(path string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var allowed []string

	for method, m := range r.static {
		if _, ok := m[path]; ok {
			if !slices.Contains(allowed, method) {
				allowed = append(allowed, method)
			}
		}
	}

	var segBuf [16]string
	segments := splitPathBuf(path, &segBuf)
	hasTrailingSlash := len(path) > 1 && strings.HasSuffix(path, "/")

	for method, root := range r.routes {
		if slices.Contains(allowed, method) {
			continue
		}

		if n, ok := matchNode(root, segments, hasTrailingSlash, nil); ok && n != nil && n.handler != nil {
			allowed = append(allowed, method)
		}
	}

	slices.Sort(allowed)
	return allowed
}

// HasPath returns true if any HTTP method is registered for the specified path.
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
		if n, ok := matchNode(root, segments, hasTrailingSlash, nil); ok && n != nil && n.handler != nil {
			return true
		}
	}

	return false
}

// Routes returns an immutable slice of all registered [RouteInfo] metadata entries.
func (r *Router) Routes() []RouteInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return slices.Clone(r.routeList)
}

func splitPathBuf(path string, buf *[16]string) []string {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil
	}

	var count int
	start := 0

	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			if i > start {
				if count < len(buf) {
					buf[count] = path[start:i]
					count++
				}
			}
			start = i + 1
		}
	}

	if start < len(path) && count < len(buf) {
		buf[count] = path[start:]
		count++
	}

	res := make([]string, count)
	copy(res, buf[:count])

	return res
}
