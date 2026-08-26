// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package healthcheck provides Kubernetes liveness and readiness probe middleware
// returning structured JSON health metrics.
package healthcheck

import (
	"context"
	"net/http"
	"time"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// Checker represents a health validation function (e.g. database ping, redis ping).
type Checker func(ctx context.Context) error

// HealthStatus represents the structured probe output payload.
type HealthStatus struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// Config configures the Healthcheck middleware.
type Config struct {
	// LivenessPath is the endpoint for Kubernetes liveness probe. Default is "/livez".
	LivenessPath string
	// ReadinessPath is the endpoint for Kubernetes readiness probe. Default is "/readyz".
	ReadinessPath string
	// LiveCheckers defines components validated during liveness probes.
	LiveCheckers map[string]Checker
	// ReadyCheckers defines components validated during readiness probes.
	ReadyCheckers map[string]Checker
}

// Option configures Healthcheck settings.
type Option func(*Config)

// WithLivenessPath sets the liveness probe endpoint path.
func WithLivenessPath(path string) Option {
	return func(c *Config) {
		c.LivenessPath = path
	}
}

// WithReadinessPath sets the readiness probe endpoint path.
func WithReadinessPath(path string) Option {
	return func(c *Config) {
		c.ReadinessPath = path
	}
}

// WithLiveChecker registers a component checker for liveness probes.
func WithLiveChecker(name string, checker Checker) Option {
	return func(c *Config) {
		if c.LiveCheckers == nil {
			c.LiveCheckers = make(map[string]Checker)
		}

		c.LiveCheckers[name] = checker
	}
}

// WithReadyChecker registers a component checker for readiness probes.
func WithReadyChecker(name string, checker Checker) Option {
	return func(c *Config) {
		if c.ReadyCheckers == nil {
			c.ReadyCheckers = make(map[string]Checker)
		}

		c.ReadyCheckers[name] = checker
	}
}

// New creates a Healthcheck probe middleware intercepting liveness and readiness paths.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		LivenessPath:  "/livez",
		ReadinessPath: "/readyz",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			path := req.Path()

			if path == cfg.LivenessPath {
				return executeChecks(req.Context(), cfg.LiveCheckers)
			}

			if path == cfg.ReadinessPath {
				return executeChecks(req.Context(), cfg.ReadyCheckers)
			}

			return next(req)
		}
	}
}

// Register registers health check probe endpoints directly on the server.
func Register(app *sein.Server, opts ...Option) {
	cfg := Config{
		LivenessPath:  "/livez",
		ReadinessPath: "/readyz",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	app.GetReq(cfg.LivenessPath, func(req *sein.Request) (any, error) {
		return executeChecks(req.Context(), cfg.LiveCheckers)
	})

	app.GetReq(cfg.ReadinessPath, func(req *sein.Request) (any, error) {
		return executeChecks(req.Context(), cfg.ReadyCheckers)
	})
}

func executeChecks(ctx context.Context, checkers map[string]Checker) (any, error) {
	status := "UP"
	httpStatus := http.StatusOK
	results := make(map[string]string, len(checkers))

	for name, check := range checkers {
		if check != nil {
			if err := check(ctx); err != nil {
				status = "DOWN"
				httpStatus = http.StatusServiceUnavailable
				results[name] = "DOWN: " + err.Error()
			} else {
				results[name] = "OK"
			}
		}
	}

	payload := HealthStatus{
		Status:    status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    results,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return sein.OK[any](data).
		WithStatus(httpStatus).
		WithHeader(header.ContentType, header.MIMEApplicationJSONCharsetUTF8), nil
}
