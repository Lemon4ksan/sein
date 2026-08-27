// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package timeout provides request execution deadline middleware, returning HTTP 504 Gateway Timeout
// when handler processing exceeds the allotted time boundary.
package timeout

import (
	"context"
	"time"

	"github.com/lemon4ksan/sein"
)

// Config configures the Timeout middleware.
type Config struct {
	// Timeout is the maximum execution duration permitted for each request. Default is 30s.
	Timeout time.Duration
	// ErrorHandler is the rejection handler invoked when execution times out. Default returns HTTP 504.
	ErrorHandler func(req *sein.Request) (any, error)
}

// Option configures Timeout settings.
type Option func(*Config)

// WithTimeout sets the request execution duration threshold.
func WithTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.Timeout = d
	}
}

// WithErrorHandler overrides the rejection handler on timeout expiration.
func WithErrorHandler(handler func(req *sein.Request) (any, error)) Option {
	return func(c *Config) {
		c.ErrorHandler = handler
	}
}

type handlerResult struct {
	res any
	err error
}

// New creates a request execution deadline middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		Timeout: 30 * time.Second,
		ErrorHandler: func(_ *sein.Request) (any, error) {
			return nil, sein.ErrGatewayTimeout("request context deadline exceeded")
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			ctx, cancel := context.WithTimeout(req.Context(), cfg.Timeout)
			defer cancel()

			req.SetContext(ctx)

			done := make(chan handlerResult, 1)

			go func() {
				res, err := next(req)
				done <- handlerResult{res: res, err: err}
			}()

			select {
			case <-ctx.Done():
				req.Detach()
				return cfg.ErrorHandler(req)
			case r := <-done:
				return r.res, r.err
			}
		}
	}
}
