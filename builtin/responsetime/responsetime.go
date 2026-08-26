// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package responsetime provides HTTP response latency measurement middleware
// injecting X-Response-Time and W3C Server-Timing headers.
package responsetime

import (
	"fmt"

	"github.com/lemon4ksan/foundation/timekit"

	"github.com/lemon4ksan/sein"
)

// DefaultHeader is the standard header name for response time.
const DefaultHeader = "X-Response-Time"

// Config configures the ResponseTime middleware.
type Config struct {
	// Header is the response header key. Default is "X-Response-Time".
	Header string
	// ServerTiming controls whether the W3C Server-Timing header is also injected. Default is false.
	ServerTiming bool
}

// Option configures ResponseTime settings.
type Option func(*Config)

// WithHeader overrides the response time header name.
func WithHeader(name string) Option {
	return func(c *Config) {
		c.Header = name
	}
}

// WithServerTiming enables W3C Server-Timing header injection.
func WithServerTiming(enabled bool) Option {
	return func(c *Config) {
		c.ServerTiming = enabled
	}
}

// New creates a response time header injection middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		Header: DefaultHeader,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			sw := timekit.StartStopwatch()
			res, err := next(req)
			latency := sw.Elapsed()

			durationStr := fmt.Sprintf("%.2fms", float64(latency.Nanoseconds())/1e6)

			if holder, ok := res.(sein.ResponseHolder); ok {
				resp := sein.OK[any](holder.ResponseBody()).
					WithStatus(holder.StatusCode()).
					WithHeaders(holder.ResponseHeaders()).
					WithHeader(cfg.Header, durationStr)

				if cfg.ServerTiming {
					resp = resp.WithHeader("Server-Timing", fmt.Sprintf("total;dur=%.2f", float64(latency.Nanoseconds())/1e6))
				}

				return resp, err
			}

			resp := sein.OK[any](res).WithHeader(cfg.Header, durationStr)
			if cfg.ServerTiming {
				resp = resp.WithHeader("Server-Timing", fmt.Sprintf("total;dur=%.2f", float64(latency.Nanoseconds())/1e6))
			}

			return resp, err
		}
	}
}
