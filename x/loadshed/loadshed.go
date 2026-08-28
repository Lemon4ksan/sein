// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package loadshed provides adaptive load shedding and concurrency-limiting middleware
// protecting backend services from thundering herds, latency spikes, and out-of-memory crashes.
package loadshed

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/generic"

	"github.com/lemon4ksan/sein"
)

// Config configures the LoadShedding middleware.
type Config struct {
	// MaxInFlight is the maximum number of concurrent requests allowed before shedding occurs. Default is 5000.
	MaxInFlight int64
	// MaxLatencyThreshold is the moving average latency threshold that triggers load shedding. Default is 500ms.
	MaxLatencyThreshold time.Duration
	// ErrorHandler is the rejection handler on shed requests. Default returns HTTP 503 Service Unavailable.
	ErrorHandler func(req *sein.Request) (any, error)
}

// Option configures LoadShedding settings.
type Option func(*Config)

// WithMaxInFlight sets maximum concurrent active in-flight requests.
func WithMaxInFlight(max int64) Option {
	return func(c *Config) {
		c.MaxInFlight = max
	}
}

// WithMaxLatencyThreshold sets the moving latency threshold.
func WithMaxLatencyThreshold(d time.Duration) Option {
	return func(c *Config) {
		c.MaxLatencyThreshold = d
	}
}

// WithErrorHandler overrides the rejection handler.
func WithErrorHandler(handler func(req *sein.Request) (any, error)) Option {
	return func(c *Config) {
		c.ErrorHandler = handler
	}
}

// New creates an adaptive load-shedding middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		MaxInFlight:         5000,
		MaxLatencyThreshold: 500 * time.Millisecond,
		ErrorHandler: func(_ *sein.Request) (any, error) {
			return nil, sein.NewHTTPError(http.StatusServiceUnavailable, "SERVER_OVERLOADED", "server is currently overloaded, please retry later")
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	var (
		inFlight  atomic.Int64
		avgMicros atomic.Uint64 // EWMA latency in microseconds
	)

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			current := inFlight.Add(1)
			defer inFlight.Add(-1)

			// 1. Check in-flight ceiling
			if cfg.MaxInFlight > 0 && current > cfg.MaxInFlight {
				return cfg.ErrorHandler(req)
			}

			// 2. Check latency threshold if moving average has data
			if cfg.MaxLatencyThreshold > 0 {
				avg := time.Duration(avgMicros.Load()) * time.Microsecond
				if avg > cfg.MaxLatencyThreshold && current > 10 {
					return cfg.ErrorHandler(req)
				}
			}

			start := time.Now()
			res, err := next(req)
			duration := time.Since(start)

			// Update EWMA (alpha = 0.1)
			durMicros := uint64(duration.Microseconds())
			oldAvg := avgMicros.Load()
			newAvg := generic.Ternary(oldAvg == 0, durMicros, uint64(float64(oldAvg)*0.9+float64(durMicros)*0.1))
			avgMicros.Store(newAvg)

			return res, err
		}
	}
}
