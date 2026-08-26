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
	if !strings.Contains(pattern, ":") && !strings.Contains(pattern, "*") {
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

		isParam := strings.HasPrefix(seg, ":")
		paramName := ""
		if isParam {
			paramName = seg[1:]
		}

		var child *routeNode
		for _, c := range curr.children {
			if isParam && c.isParam && c.paramName == paramName {
				child = c
				break
			}
			if !isParam && !c.isParam && c.pathSegment == seg {
				child = c
				break
			}
		}

		if child == nil {
			child = &routeNode{
				pathSegment: seg,
				isParam:     isParam,
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
		return nil, false
	}

	seg := segments[0]
	remaining := segments[1:]

	// 1. Try exact match first
	for _, child := range curr.children {
		if !child.isParam && child.pathSegment == seg {
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

	return nil, false
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}
