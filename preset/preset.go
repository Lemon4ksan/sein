// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package preset provides ready-to-use production server presets and consolidated middleware suites,
// allowing applications to configure enterprise security, metrics, and compression with a single import.
package preset

import (
	"time"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/cache"
	"github.com/lemon4ksan/sein/builtin/compress"
	"github.com/lemon4ksan/sein/builtin/cors"
	"github.com/lemon4ksan/sein/builtin/etag"
	"github.com/lemon4ksan/sein/builtin/helmet"
	"github.com/lemon4ksan/sein/builtin/recover"
	"github.com/lemon4ksan/sein/builtin/responsetime"
	"github.com/lemon4ksan/sein/builtin/revision"
	"github.com/lemon4ksan/sein/x/loadshed"
	"github.com/lemon4ksan/sein/x/prometheus"
)

// CORSConfig is an alias to the standard CORS configuration.
type CORSConfig = cors.Config

// RevisionInfo holds release and build metadata.
type RevisionInfo = revision.Info

// Options configures the preset configuration.
type Options struct {
	CORS         *cors.Config
	Prometheus   string
	Revision     string
	RevisionPath string
	LoadShedding time.Duration
}

// Option configures production presets.
type Option func(*Options)

// WithCORS configures Cross-Origin Resource Sharing.
func WithCORS(cfg cors.Config) Option {
	return func(o *Options) {
		o.CORS = &cfg
	}
}

// WithPrometheus enables Prometheus metrics on the given path (default "/metrics").
func WithPrometheus(path ...string) Option {
	return func(o *Options) {
		if len(path) > 0 && path[0] != "" {
			o.Prometheus = path[0]
		} else {
			o.Prometheus = "/metrics"
		}
	}
}

// WithRevision enables release version reporting and diagnostics endpoint.
func WithRevision(version string, path ...string) Option {
	return func(o *Options) {
		o.Revision = version
		if len(path) > 0 && path[0] != "" {
			o.RevisionPath = path[0]
		} else {
			o.RevisionPath = "/version"
		}
	}
}

// WithLoadShedding enables adaptive load shedding protection.
func WithLoadShedding(targetLatency time.Duration) Option {
	return func(o *Options) {
		o.LoadShedding = targetLatency
	}
}

// Apply configures an existing Sein server with the standard high-performance production stack.
func Apply(s *sein.Server, opts ...Option) *sein.Server {
	var o Options
	for _, opt := range opts {
		opt(&o)
	}

	// 1. Recover panic protection
	s.Use(recover.New())

	// 2. A+ Security headers
	s.Use(helmet.New())

	// 3. CORS
	if o.CORS != nil {
		s.Use(cors.New(*o.CORS))
	}

	// 4. Response Time
	s.Use(responsetime.New())

	// 5. Transparent Compression
	s.Use(compress.New())

	// 6. Adaptive Load Shedding
	if o.LoadShedding > 0 {
		s.Use(loadshed.New(loadshed.WithMaxLatencyThreshold(o.LoadShedding)))
	} else {
		s.Use(loadshed.New(loadshed.WithMaxLatencyThreshold(50 * time.Millisecond)))
	}

	// 7. Prometheus Exporter
	if o.Prometheus != "" {
		prometheus.Register(s, prometheus.WithMetricsPath(o.Prometheus))
	}

	// 8. Revision diagnostics
	if o.Revision != "" {
		revPath := "/version"
		if o.RevisionPath != "" {
			revPath = o.RevisionPath
		}
		revision.Register(s, revision.WithVersion(o.Revision), revision.WithPath(revPath))
	}

	return s
}

// Production creates a new Sein server pre-configured with the full production stack.
func Production(opts ...Option) *sein.Server {
	s := sein.New()
	Apply(s, opts...)

	return s
}

// Recover returns a zero-allocation panic recovery middleware.
func Recover() sein.Middleware {
	return recover.New()
}

// Helmet returns an A+ security headers middleware.
func Helmet() sein.Middleware {
	return helmet.New()
}

// Compress returns a transparent compression middleware.
func Compress() sein.Middleware {
	return compress.New()
}

// ResponseTime returns a response time tracking middleware.
func ResponseTime() sein.Middleware {
	return responsetime.New()
}

// CORS returns a CORS middleware.
func CORS(cfg ...cors.Config) sein.Middleware {
	return cors.New(cfg...)
}

// Cache returns an RFC 7234 response caching middleware with TTL.
func Cache(ttl time.Duration) sein.Middleware {
	return cache.Middleware(cache.WithExpiration(ttl))
}

// ETag returns an RFC 7232 ETag conditional request middleware.
func ETag() sein.Middleware {
	return etag.New()
}
