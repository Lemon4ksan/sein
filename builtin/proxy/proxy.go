// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package proxy provides high-throughput HTTP reverse proxy and load balancing middleware
// forwarding requests to upstream backend servers.
package proxy

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// Config configures the Reverse Proxy middleware.
type Config struct {
	// Targets is the list of upstream backend URLs (e.g. "http://127.0.0.1:8081").
	Targets []string
	// Client is the underlying HTTP transport client.
	Client *http.Client
	// ModifyRequest is an optional callback to transform upstream requests.
	ModifyRequest func(req *http.Request)
	// ErrorHandler is invoked when upstream communication fails. Default returns HTTP 502 Bad Gateway.
	ErrorHandler func(req *sein.Request, err error) (any, error)
}

// Option configures Proxy settings.
type Option func(*Config)

// WithTargets sets upstream backend target URLs.
func WithTargets(targets ...string) Option {
	return func(c *Config) {
		c.Targets = append(c.Targets, targets...)
	}
}

// WithClient sets a custom HTTP client.
func WithClient(client *http.Client) Option {
	return func(c *Config) {
		c.Client = client
	}
}

// WithModifyRequest sets a request transformer callback.
func WithModifyRequest(fn func(req *http.Request)) Option {
	return func(c *Config) {
		c.ModifyRequest = fn
	}
}

// WithErrorHandler overrides the upstream connection error handler.
func WithErrorHandler(handler func(req *sein.Request, err error) (any, error)) Option {
	return func(c *Config) {
		c.ErrorHandler = handler
	}
}

// New creates an HTTP reverse proxy middleware distributing requests among targets in round-robin order.
func New(opts ...Option) sein.Middleware {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}

	cfg := Config{
		Client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		ErrorHandler: func(_ *sein.Request, err error) (any, error) {
			return nil, sein.NewHTTPError(http.StatusBadGateway, "BAD_GATEWAY", "upstream server error: "+err.Error())
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	var counter atomic.Uint64

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			if len(cfg.Targets) == 0 {
				return next(req)
			}

			idx := int(counter.Add(1)-1) % len(cfg.Targets)
			targetBase := strings.TrimSuffix(cfg.Targets[idx], "/")

			fullURL := targetBase + req.Path()
			if q := req.Query(""); q != "" {
				fullURL += "?" + string(q)
			}

			bodyReader := bytes.NewReader(req.RawBody())
			proxyReq, err := http.NewRequestWithContext(req.Context(), req.Method(), fullURL, bodyReader)
			if err != nil {
				return cfg.ErrorHandler(req, err)
			}

			// Forward request headers
			if raw := req.Raw(); raw != nil {
				proxyReq.Header = raw.Header.Clone()
			}

			// Standard proxy headers
			clientIP := req.ClientIP()
			if prior := proxyReq.Header.Get(header.XForwardedFor); prior != "" {
				proxyReq.Header.Set(header.XForwardedFor, prior+", "+clientIP)
			} else {
				proxyReq.Header.Set(header.XForwardedFor, clientIP)
			}

			proxyReq.Header.Set(header.XForwardedProto, req.Scheme())
			proxyReq.Header.Set(header.XForwardedHost, req.Host())

			if cfg.ModifyRequest != nil {
				cfg.ModifyRequest(proxyReq)
			}

			resp, err := cfg.Client.Do(proxyReq)
			if err != nil {
				return cfg.ErrorHandler(req, err)
			}
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				return cfg.ErrorHandler(req, err)
			}

			out := sein.OK[any](respBody).WithStatus(resp.StatusCode)
			for k, vv := range resp.Header {
				for _, v := range vv {
					out = out.WithHeader(k, v)
				}
			}

			return out, nil
		}
	}
}
