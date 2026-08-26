// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package earlydata provides HTTP/2, HTTP/3, and TLS 1.3 0-RTT Anti-Replay protection
// complying with RFC 8470 (Using Early Data in HTTP).
package earlydata

import (
	"slices"
	"strings"

	"github.com/lemon4ksan/sein"
)

// HeaderEarlyData is the canonical RFC 8470 header name.
const HeaderEarlyData = "Early-Data"

// Config configures the RFC 8470 Early Data anti-replay middleware.
type Config struct {
	// AllowSafeOnly restricts early data requests strictly to safe methods (GET, HEAD, OPTIONS, TRACE). Default is true.
	AllowSafeOnly bool
	// AllowPaths defines specific request path prefixes permitted to execute in early data regardless of method.
	AllowPaths []string
	// AllowFunc is an optional custom predicate for dynamic early data authorization.
	AllowFunc func(req *sein.Request) bool
	// ErrorHandler is the handler called when an unsafe early data request is rejected. Default returns HTTP 425 Too Early.
	ErrorHandler func(req *sein.Request) (any, error)
}

// Option configures Early Data settings.
type Option func(*Config)

// WithAllowSafeOnly sets whether only safe HTTP methods are allowed.
func WithAllowSafeOnly(safeOnly bool) Option {
	return func(c *Config) {
		c.AllowSafeOnly = safeOnly
	}
}

// WithAllowPaths adds permitted path prefixes for early data execution.
func WithAllowPaths(paths ...string) Option {
	return func(c *Config) {
		c.AllowPaths = append(c.AllowPaths, paths...)
	}
}

// WithAllowFunc configures a custom early data authorization callback.
func WithAllowFunc(fn func(req *sein.Request) bool) Option {
	return func(c *Config) {
		c.AllowFunc = fn
	}
}

// WithErrorHandler overrides the rejection error handler.
func WithErrorHandler(handler func(req *sein.Request) (any, error)) Option {
	return func(c *Config) {
		c.ErrorHandler = handler
	}
}

var safeMethods = []string{"GET", "HEAD", "OPTIONS", "TRACE"}

// New creates an RFC 8470 Early Data anti-replay middleware.
// It inspects the "Early-Data" header and rejects non-idempotent or mutating methods
// with status HTTP 425 Too Early, allowing the client to safely retry upon handshake completion.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		AllowSafeOnly: true,
		ErrorHandler: func(req *sein.Request) (any, error) {
			return nil, sein.ErrTooEarly("request received in early data cannot be safely processed")
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			// RFC 8470 §2: "Early-Data: 1" signals an early data request
			if req.Header(HeaderEarlyData) != "1" {
				return next(req)
			}

			// Custom predicate
			if cfg.AllowFunc != nil && cfg.AllowFunc(req) {
				return next(req)
			}

			// Whitelisted paths
			path := req.Path()
			for _, allowed := range cfg.AllowPaths {
				if strings.HasPrefix(path, allowed) {
					return next(req)
				}
			}

			// Check method safety
			if cfg.AllowSafeOnly {
				method := strings.ToUpper(req.Method())
				if slices.Contains(safeMethods, method) {
					return next(req)
				}

				return cfg.ErrorHandler(req)
			}

			return next(req)
		}
	}
}
