// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"strings"
	"sync"
)

type routeNode struct {
	pathSegment string
	isParam     bool
	isWildcard  bool
	paramName   string
	handler     RawHandler
	children    []*routeNode
}

type Router struct {
	mu     sync.RWMutex
	routes map[string]*routeNode
	static map[string]map[string]RawHandler
}

func NewRouter() *Router {
	return &Router{
		routes: make(map[string]*routeNode),
		static: make(map[string]map[string]RawHandler),
	}
}

func (r *Router) Add(method, pattern string, handler RawHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

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
}

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

	handler, ok := matchNode(root, segments, params)
	if !ok {
		return nil, nil, false
	}

	return handler, params, true
}

func matchNode(curr *routeNode, segments []string, params map[string]string) (RawHandler, bool) {
	if len(segments) == 0 {
		if curr.handler != nil {
			return curr.handler, true
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
			if h, ok := matchNode(child, remaining, params); ok {
				return h, true
			}
		}
	}

	// 2. Try param match
	for _, child := range curr.children {
		if child.isParam {
			params[child.paramName] = seg
			if h, ok := matchNode(child, remaining, params); ok {
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

// HasPath returns true if any HTTP method is registered for the path.
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
	for _, root := range r.routes {
		if _, ok := matchNode(root, segments, params); ok {
			return true
		}
	}

	return false
}
