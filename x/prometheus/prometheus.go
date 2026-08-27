// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package prometheus provides zero-dependency Prometheus metrics collection and exposition middleware,
// serving request latency histograms, status code counters, and runtime gauges on /metrics.
package prometheus

import (
	"bytes"
	"fmt"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// DefaultMetricsPath is the standard Prometheus metrics endpoint path.
const DefaultMetricsPath = "/metrics"

var defaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0}

type metricKey struct {
	method string
	path   string
	status int
}

// Registry stores aggregated Prometheus metrics with thread-safe counters.
type Registry struct {
	mu          sync.RWMutex
	counters    map[metricKey]*atomic.Uint64
	histograms  map[metricKey]*histogram
	inFlight    atomic.Int64
	buckets     []float64
	metricsPath string
}

type histogram struct {
	buckets []atomic.Uint64
	count   atomic.Uint64
	sum     atomic.Uint64 // float64 stored as integer microsecond sum
}

// NewRegistry creates a new Prometheus metrics registry.
func NewRegistry(metricsPath string, buckets []float64) *Registry {
	if len(buckets) == 0 {
		buckets = defaultBuckets
	}

	return &Registry{
		counters:    make(map[metricKey]*atomic.Uint64),
		histograms:  make(map[metricKey]*histogram),
		buckets:     buckets,
		metricsPath: metricsPath,
	}
}

func (r *Registry) observe(method, path string, status int, durationSec float64) {
	key := metricKey{method: method, path: path, status: status}

	r.mu.RLock()
	c, hasC := r.counters[key]
	h, hasH := r.histograms[key]
	r.mu.RUnlock()

	if !hasC || !hasH {
		r.mu.Lock()
		if c = r.counters[key]; c == nil {
			c = &atomic.Uint64{}
			r.counters[key] = c
		}

		if h = r.histograms[key]; h == nil {
			h = &histogram{
				buckets: make([]atomic.Uint64, len(r.buckets)),
			}
			r.histograms[key] = h
		}
		r.mu.Unlock()
	}

	c.Add(1)
	h.count.Add(1)
	h.sum.Add(uint64(durationSec * 1e6))

	for i, b := range r.buckets {
		if durationSec <= b {
			h.buckets[i].Add(1)
		}
	}
}

// Gather serializes all metrics into Prometheus standard text format.
func (r *Registry) Gather() []byte {
	var buf bytes.Buffer

	buf.WriteString("# HELP http_requests_in_flight Current number of active HTTP requests.\n")
	buf.WriteString("# TYPE http_requests_in_flight gauge\n")
	buf.WriteString("http_requests_in_flight " + strconv.FormatInt(r.inFlight.Load(), 10) + "\n\n")

	// Runtime gauges
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	buf.WriteString("# HELP go_goroutines Number of active goroutines.\n")
	buf.WriteString("# TYPE go_goroutines gauge\n")
	buf.WriteString("go_goroutines " + strconv.Itoa(runtime.NumGoroutine()) + "\n\n")

	buf.WriteString("# HELP process_resident_memory_bytes Resident memory size in bytes.\n")
	buf.WriteString("# TYPE process_resident_memory_bytes gauge\n")
	buf.WriteString("process_resident_memory_bytes " + strconv.FormatUint(mem.Sys, 10) + "\n\n")

	buf.WriteString("# HELP http_requests_total Total number of HTTP requests processed.\n")
	buf.WriteString("# TYPE http_requests_total counter\n")

	r.mu.RLock()
	for key, cnt := range r.counters {
		fmt.Fprintf(&buf, "http_requests_total{method=\"%s\",path=\"%s\",status=\"%d\"} %d\n",
			key.method, key.path, key.status, cnt.Load())
	}

	buf.WriteString("\n# HELP http_request_duration_seconds HTTP request latency histogram.\n")
	buf.WriteString("# TYPE http_request_duration_seconds histogram\n")

	for key, h := range r.histograms {
		var cumulative uint64
		for i, b := range r.buckets {
			cumulative += h.buckets[i].Load()
			fmt.Fprintf(&buf, "http_request_duration_seconds_bucket{method=\"%s\",path=\"%s\",status=\"%d\",le=\"%g\"} %d\n",
				key.method, key.path, key.status, b, cumulative)
		}

		fmt.Fprintf(&buf, "http_request_duration_seconds_bucket{method=\"%s\",path=\"%s\",status=\"%d\",le=\"+Inf\"} %d\n",
			key.method, key.path, key.status, h.count.Load())
		fmt.Fprintf(&buf, "http_request_duration_seconds_sum{method=\"%s\",path=\"%s\",status=\"%d\"} %g\n",
			key.method, key.path, key.status, float64(h.sum.Load())/1e6)
		fmt.Fprintf(&buf, "http_request_duration_seconds_count{method=\"%s\",path=\"%s\",status=\"%d\"} %d\n",
			key.method, key.path, key.status, h.count.Load())
	}
	r.mu.RUnlock()

	return buf.Bytes()
}

// Config configures the Prometheus middleware.
type Config struct {
	// MetricsPath is the endpoint where Prometheus metrics are exposed. Default is "/metrics".
	MetricsPath string
	// Buckets defines the histogram latency buckets in seconds.
	Buckets []float64
	// IgnorePaths defines paths excluded from metric tracking.
	IgnorePaths []string
}

// Option configures Prometheus settings.
type Option func(*Config)

// WithMetricsPath sets the metrics exposition path.
func WithMetricsPath(path string) Option {
	return func(c *Config) {
		c.MetricsPath = path
	}
}

// WithBuckets sets custom histogram latency buckets.
func WithBuckets(buckets []float64) Option {
	return func(c *Config) {
		c.Buckets = buckets
	}
}

// WithIgnorePaths adds paths to ignore from tracking.
func WithIgnorePaths(paths ...string) Option {
	return func(c *Config) {
		c.IgnorePaths = append(c.IgnorePaths, paths...)
	}
}

// New creates a Prometheus metrics middleware.
func New(opts ...Option) (sein.Middleware, *Registry) {
	cfg := Config{
		MetricsPath: DefaultMetricsPath,
		Buckets:     defaultBuckets,
		IgnorePaths: []string{"/metrics", "/favicon.ico"},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	reg := NewRegistry(cfg.MetricsPath, cfg.Buckets)

	mw := func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			path := req.Path()

			if path == cfg.MetricsPath {
				return sein.OK[any](reg.Gather()).
					WithHeader(header.ContentType, "text/plain; version=0.0.4; charset=utf-8"), nil
			}

			if slices.Contains(cfg.IgnorePaths, path) {
				return next(req)
			}

			reg.inFlight.Add(1)
			start := time.Now()

			res, err := next(req)
			duration := time.Since(start).Seconds()
			reg.inFlight.Add(-1)

			status := http.StatusOK
			if err != nil {
				if domainErr, ok := err.(sein.DomainError); ok {
					status = domainErr.HTTPStatus()
				} else if httpErr, ok := sein.AsHTTPError(err); ok {
					status = httpErr.HTTPStatus()
				} else {
					status = http.StatusInternalServerError
				}
			} else if holder, ok := res.(sein.ResponseHolder); ok {
				if code := holder.StatusCode(); code != 0 {
					status = code
				}
			}

			reg.observe(req.Method(), req.RoutePattern(), status, duration)

			return res, err
		}
	}

	return mw, reg
}

// Register attaches the metrics middleware and /metrics route handler directly onto the sein server.
func Register(app *sein.Server, opts ...Option) *Registry {
	mw, reg := New(opts...)
	app.Use(mw)

	app.Get(reg.metricsPath, func(_ *sein.Request) (any, error) {
		return sein.OK[any](reg.Gather()).
			WithHeader(header.ContentType, "text/plain; version=0.0.4; charset=utf-8"), nil
	})

	return reg
}
