// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package recover provides panic recovery middleware for sein HTTP pipelines.
// It intercepts unhandled runtime panics, logs structured stack traces via [log.Logger],
// and returns a safe HTTP 500 Internal Server Error response.
package recover

import (
	"runtime/debug"

	"github.com/lemon4ksan/foundation/async/log"

	"github.com/lemon4ksan/sein"
)

// Config configures the panic recovery middleware.
type Config struct {
	// Logger is the structured logger used to record panic events. Default is log.Default().
	Logger log.Logger
	// EnableStackTrace controls whether full stack traces are logged on panic. Default is true.
	EnableStackTrace bool
	// ErrorHandler is the custom error handler invoked when a panic occurs. Default returns HTTP 500.
	ErrorHandler func(req *sein.Request, err any) (any, error)
}

// Option configures panic recovery settings.
type Option func(*Config)

// WithLogger sets the structured logger instance.
func WithLogger(l log.Logger) Option {
	return func(c *Config) {
		c.Logger = l
	}
}

// WithStackTrace configures whether stack traces are logged.
func WithStackTrace(enabled bool) Option {
	return func(c *Config) {
		c.EnableStackTrace = enabled
	}
}

// WithErrorHandler overrides the panic recovery response handler.
func WithErrorHandler(handler func(req *sein.Request, err any) (any, error)) Option {
	return func(c *Config) {
		c.ErrorHandler = handler
	}
}

// New creates a panic recovery middleware.
// It catches unhandled panics anywhere in the downstream middleware or handler chain,
// safely converts them into structured domain errors, and prevents process crashes.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		Logger:           log.New(log.DefaultConfig(log.LevelInfo)),
		EnableStackTrace: true,
		ErrorHandler: func(_ *sein.Request, _ any) (any, error) {
			return nil, sein.ErrInternal("an unexpected panic occurred")
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (res any, err error) {
			defer func() {
				if r := recover(); r != nil {
					if cfg.Logger != nil {
						if cfg.EnableStackTrace {
							cfg.Logger.Error("panic recovered in HTTP handler",
								log.String("path", req.Path()),
								log.String("method", req.Method()),
								log.Any("error", r),
								log.String("stack", string(debug.Stack())),
							)
						} else {
							cfg.Logger.Error("panic recovered in HTTP handler",
								log.String("path", req.Path()),
								log.String("method", req.Method()),
								log.Any("error", r),
							)
						}
					}

					res, err = cfg.ErrorHandler(req, r)
				}
			}()

			return next(req)
		}
	}
}
