// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package circuitbreaker provides fault-tolerance middleware protecting upstream services
// from cascading downstream failures using a Closed -> Open -> Half-Open state machine.
package circuitbreaker

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/lemon4ksan/foundation/sync/breaker"

	"github.com/lemon4ksan/sein"
)

// State re-exports operational states from foundation/sync/breaker.
type State = breaker.State

// Re-exported operational circuit breaker states.
const (
	StateClosed   = breaker.StateClosed
	StateOpen     = breaker.StateOpen
	StateHalfOpen = breaker.StateHalfOpen
)

// Config configures the CircuitBreaker middleware.
type Config struct {
	// FailureThreshold is the failure ratio (0.0 to 1.0) triggering Open state. Default is 0.5 (50%).
	FailureThreshold float64
	// Cooldown is the duration spent in Open state before probing Half-Open recovery. Default is 5s.
	Cooldown time.Duration
	// MinRequests is the minimum request count within the window before threshold is checked. Default is 5.
	MinRequests int
	// Window is the sliding time duration over which failures are measured. Default is 10s.
	Window time.Duration
	// OnStateChange is an optional callback invoked on state transitions.
	OnStateChange func(from, to State)
	// ErrorHandler is invoked when requests are rejected due to an Open circuit. Default returns HTTP 503.
	ErrorHandler func(req *sein.Request) (any, error)
}

// Option configures CircuitBreaker settings.
type Option func(*Config)

// WithFailureThreshold sets the failure ratio threshold.
func WithFailureThreshold(threshold float64) Option {
	return func(c *Config) {
		c.FailureThreshold = threshold
	}
}

// WithCooldown sets the duration spent in Open state.
func WithCooldown(d time.Duration) Option {
	return func(c *Config) {
		c.Cooldown = d
	}
}

// WithMinRequests sets the minimum sample size in a window.
func WithMinRequests(n int) Option {
	return func(c *Config) {
		c.MinRequests = n
	}
}

// WithWindow sets the sliding measurement window.
func WithWindow(d time.Duration) Option {
	return func(c *Config) {
		c.Window = d
	}
}

// WithOnStateChange configures a state change listener callback.
func WithOnStateChange(fn func(from, to State)) Option {
	return func(c *Config) {
		c.OnStateChange = fn
	}
}

// WithErrorHandler overrides the rejection handler for open circuit state.
func WithErrorHandler(handler func(req *sein.Request) (any, error)) Option {
	return func(c *Config) {
		c.ErrorHandler = handler
	}
}

// New creates a CircuitBreaker middleware.
// When consecutive downstream failures exceed configured thresholds, the breaker opens,
// immediately rejecting subsequent requests with HTTP 503 Service Unavailable to allow recovery.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		FailureThreshold: 0.5,
		Cooldown:         5 * time.Second,
		MinRequests:      5,
		Window:           10 * time.Second,
		ErrorHandler: func(_ *sein.Request) (any, error) {
			return nil, sein.NewHTTPError(http.StatusServiceUnavailable, "CIRCUIT_OPEN", "service temporarily unavailable")
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	cb := breaker.New[any](breaker.Config{
		FailureThreshold: cfg.FailureThreshold,
		Cooldown:         cfg.Cooldown,
		MinRequests:      cfg.MinRequests,
		Window:           cfg.Window,
		OnStateChange:    cfg.OnStateChange,
	})

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			res, err := cb.Do(req.Context(), func(_ context.Context) (any, error) {
				return next(req)
			})
			if err != nil {
				if errors.Is(err, breaker.ErrCircuitOpen) {
					return cfg.ErrorHandler(req)
				}

				return nil, err
			}

			return res, nil
		}
	}
}
