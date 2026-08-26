// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package methodoverride provides RFC 3875 HTTP method overriding middleware,
// allowing clients to override HTTP methods using headers (X-HTTP-Method-Override) or query/form parameters (_method).
package methodoverride

import (
	"net/http"
	"slices"
	"strings"

	"github.com/lemon4ksan/sein"
)

// DefaultHeader is the standard method override header.
const DefaultHeader = "X-HTTP-Method-Override"

// DefaultQueryParam is the standard method override query parameter.
const DefaultQueryParam = "_method"

// Config configures the MethodOverride middleware.
type Config struct {
	// Header is the request header to inspect. Default is "X-HTTP-Method-Override".
	Header string
	// QueryParam is the query string parameter to inspect. Default is "_method".
	QueryParam string
	// Methods are the HTTP methods that can be overridden. Default is ["POST"].
	Methods []string
}

// Option configures MethodOverride settings.
type Option func(*Config)

// WithHeader sets the override header name.
func WithHeader(name string) Option {
	return func(c *Config) {
		c.Header = name
	}
}

// WithQueryParam sets the override query parameter name.
func WithQueryParam(name string) Option {
	return func(c *Config) {
		c.QueryParam = name
	}
}

// WithMethods sets the HTTP methods eligible for overriding.
func WithMethods(methods ...string) Option {
	return func(c *Config) {
		c.Methods = methods
	}
}

// New creates an HTTP method overriding middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		Header:     DefaultHeader,
		QueryParam: DefaultQueryParam,
		Methods:    []string{http.MethodPost},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			if !slices.Contains(cfg.Methods, req.Method()) {
				return next(req)
			}

			var targetMethod string

			// 1. Check Header
			if cfg.Header != "" {
				targetMethod = req.Header(cfg.Header)
			}

			// 2. Check Query parameter
			if targetMethod == "" && cfg.QueryParam != "" {
				targetMethod = string(req.Query(cfg.QueryParam))
			}

			if targetMethod != "" {
				targetMethod = strings.ToUpper(strings.TrimSpace(targetMethod))
				switch targetMethod {
				case http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodOptions:
					req.SetMethod(targetMethod)
				}
			}

			return next(req)
		}
	}
}
