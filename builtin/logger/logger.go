// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package logger provides high-throughput, structured HTTP access logging middleware
// integrated with [log.Logger] from foundation/async/log.
package logger

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/lemon4ksan/foundation/async/log"
	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// Config configures the HTTP access logger middleware.
type Config struct {
	// Logger is the structured logger backend. Default is log.New(log.DefaultConfig(log.LevelInfo)).
	Logger log.Logger
	// IgnorePaths defines request paths to exclude from logging (e.g. "/health", "/favicon.ico").
	IgnorePaths []string
	// Filter is an optional custom predicate to selectively skip logging for specific requests.
	Filter func(req *sein.Request) bool
}

// Option configures logger settings.
type Option func(*Config)

// WithLogger sets the structured logger instance.
func WithLogger(l log.Logger) Option {
	return func(c *Config) {
		c.Logger = l
	}
}

// WithIgnorePaths specifies path strings to exclude from logging.
func WithIgnorePaths(paths ...string) Option {
	return func(c *Config) {
		c.IgnorePaths = append(c.IgnorePaths, paths...)
	}
}

// WithFilter configures a custom predicate for skipping logs.
func WithFilter(fn func(req *sein.Request) bool) Option {
	return func(c *Config) {
		c.Filter = fn
	}
}

// New creates an HTTP access logging middleware.
// It records request latency, HTTP method, path, status code, client IP, request ID, and errors.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		Logger:      log.New(log.DefaultConfig(log.LevelInfo)),
		IgnorePaths: []string{"/favicon.ico"},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			path := req.Path()

			// Skip ignored paths or custom filter
			if slices.Contains(cfg.IgnorePaths, path) || (cfg.Filter != nil && cfg.Filter(req)) {
				return next(req)
			}

			start := time.Now()
			res, err := next(req)
			latency := time.Since(start)

			status := http.StatusOK
			if err != nil {
				var (
					domainErr  sein.DomainError
					httpErr    sein.HTTPError
					definedErr sein.DefinedError
				)

				switch {
				case errors.As(err, &definedErr):
					status = definedErr.HTTPStatus()
				case errors.As(err, &domainErr):
					status = domainErr.HTTPStatus()
				case errors.As(err, &httpErr):
					status = httpErr.HTTPStatus()
				default:
					status = http.StatusInternalServerError
				}
			} else if holder, ok := res.(sein.ResponseHolder); ok {
				if code := holder.StatusCode(); code != 0 {
					status = code
				}
			}

			if cfg.Logger != nil {
				fields := []any{
					log.String("method", req.Method()),
					log.String("path", path),
					log.Int("status", status),
					log.Duration("latency", latency),
					log.String("ip", req.ClientIP()),
				}

				if reqID := req.Header(header.XRequestID); reqID != "" {
					fields = append(fields, log.String("req_id", reqID))
				}

				if ua := req.Header(header.UserAgent); ua != "" {
					fields = append(fields, log.String("ua", ua))
				}

				if err != nil {
					fields = append(fields, log.Any("error", err))
				}

				if status >= 500 || err != nil {
					cfg.Logger.Error("HTTP access", fields...)
				} else if status >= 400 {
					cfg.Logger.Warn("HTTP access", fields...)
				} else {
					cfg.Logger.Info("HTTP access", fields...)
				}
			}

			return res, err
		}
	}
}
