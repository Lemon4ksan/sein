// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package helmet provides HTTP security headers middleware designed to harden web applications
// against common web vulnerabilities, achieving A+ ratings on security scanners.
package helmet

import (
	"fmt"
	"net/http"

	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// Config configures the HTTP security headers.
type Config struct {
	// XSSProtection sets the X-XSS-Protection header. Default is "0" (modern browsers standard).
	XSSProtection string
	// ContentTypeNosniff enables X-Content-Type-Options: nosniff. Default is true.
	ContentTypeNosniff bool
	// XFrameOptions sets the X-Frame-Options header (e.g. "SAMEORIGIN", "DENY"). Default is "SAMEORIGIN".
	XFrameOptions string
	// HSTSMaxAge sets Strict-Transport-Security max-age in seconds. Default is 31536000 (1 year).
	HSTSMaxAge int
	// HSTSIncludeSubdomains includes subdomains in HSTS. Default is true.
	HSTSIncludeSubdomains bool
	// HSTSPreload adds the preload directive to HSTS. Default is false.
	HSTSPreload bool
	// ContentSecurityPolicy sets Content-Security-Policy. Default is empty (disabled).
	ContentSecurityPolicy string
	// ReferrerPolicy sets the Referrer-Policy header. Default is "no-referrer".
	ReferrerPolicy string
	// CrossOriginOpenerPolicy sets Cross-Origin-Opener-Policy (COOP). Default is "same-origin".
	CrossOriginOpenerPolicy string
	// CrossOriginResourcePolicy sets Cross-Origin-Resource-Policy (CORP). Default is "same-origin".
	CrossOriginResourcePolicy string
	// CrossOriginEmbedderPolicy sets Cross-Origin-Embedder-Policy (COEP). Default is "".
	CrossOriginEmbedderPolicy string
	// PermissionsPolicy sets the Permissions-Policy header. Default is "".
	PermissionsPolicy string
}

// Option configures Helmet security settings.
type Option func(*Config)

// WithXSSProtection sets the X-XSS-Protection header value.
func WithXSSProtection(val string) Option {
	return func(c *Config) {
		c.XSSProtection = val
	}
}

// WithContentTypeNosniff configures whether X-Content-Type-Options: nosniff is set.
func WithContentTypeNosniff(enabled bool) Option {
	return func(c *Config) {
		c.ContentTypeNosniff = enabled
	}
}

// WithXFrameOptions sets the X-Frame-Options header value.
func WithXFrameOptions(val string) Option {
	return func(c *Config) {
		c.XFrameOptions = val
	}
}

// WithHSTS sets Strict-Transport-Security settings.
func WithHSTS(maxAgeSeconds int, includeSubDomains, preload bool) Option {
	return func(c *Config) {
		c.HSTSMaxAge = maxAgeSeconds
		c.HSTSIncludeSubdomains = includeSubDomains
		c.HSTSPreload = preload
	}
}

// WithCSP sets Content-Security-Policy.
func WithCSP(policy string) Option {
	return func(c *Config) {
		c.ContentSecurityPolicy = policy
	}
}

// WithReferrerPolicy sets Referrer-Policy.
func WithReferrerPolicy(policy string) Option {
	return func(c *Config) {
		c.ReferrerPolicy = policy
	}
}

// WithCOOP sets Cross-Origin-Opener-Policy.
func WithCOOP(val string) Option {
	return func(c *Config) {
		c.CrossOriginOpenerPolicy = val
	}
}

// WithCORP sets Cross-Origin-Resource-Policy.
func WithCORP(val string) Option {
	return func(c *Config) {
		c.CrossOriginResourcePolicy = val
	}
}

// WithCOEP sets Cross-Origin-Embedder-Policy.
func WithCOEP(val string) Option {
	return func(c *Config) {
		c.CrossOriginEmbedderPolicy = val
	}
}

// WithPermissionsPolicy sets Permissions-Policy.
func WithPermissionsPolicy(policy string) Option {
	return func(c *Config) {
		c.PermissionsPolicy = policy
	}
}

type headerEntry struct {
	key string
	val string
}

// New creates a new Helmet security headers middleware with production-grade defaults.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		XSSProtection:             "0",
		ContentTypeNosniff:        true,
		XFrameOptions:             "SAMEORIGIN",
		HSTSMaxAge:                31536000,
		HSTSIncludeSubdomains:     true,
		ReferrerPolicy:            "no-referrer",
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "same-origin",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	var headers []headerEntry

	if cfg.ContentTypeNosniff {
		headers = append(headers, headerEntry{key: header.XContentTypeOptions, val: "nosniff"})
	}

	if cfg.XFrameOptions != "" {
		headers = append(headers, headerEntry{key: header.XFrameOptions, val: cfg.XFrameOptions})
	}

	if cfg.XSSProtection != "" {
		headers = append(headers, headerEntry{key: header.XXSSProtection, val: cfg.XSSProtection})
	}

	if cfg.HSTSMaxAge > 0 {
		hsts := fmt.Sprintf("max-age=%d", cfg.HSTSMaxAge)
		if cfg.HSTSIncludeSubdomains {
			hsts += "; includeSubDomains"
		}

		if cfg.HSTSPreload {
			hsts += "; preload"
		}

		headers = append(headers, headerEntry{key: header.StrictTransportSecurity, val: hsts})
	}

	if cfg.ReferrerPolicy != "" {
		headers = append(headers, headerEntry{key: header.ReferrerPolicy, val: cfg.ReferrerPolicy})
	}

	if cfg.ContentSecurityPolicy != "" {
		headers = append(headers, headerEntry{key: header.ContentSecurityPolicy, val: cfg.ContentSecurityPolicy})
	}

	if cfg.CrossOriginOpenerPolicy != "" {
		headers = append(headers, headerEntry{key: header.CrossOriginOpenerPolicy, val: cfg.CrossOriginOpenerPolicy})
	}

	if cfg.CrossOriginResourcePolicy != "" {
		headers = append(headers, headerEntry{key: header.CrossOriginResourcePolicy, val: cfg.CrossOriginResourcePolicy})
	}

	if cfg.CrossOriginEmbedderPolicy != "" {
		headers = append(headers, headerEntry{key: header.CrossOriginEmbedderPolicy, val: cfg.CrossOriginEmbedderPolicy})
	}

	if cfg.PermissionsPolicy != "" {
		headers = append(headers, headerEntry{key: header.PermissionsPolicy, val: cfg.PermissionsPolicy})
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			res, err := next(req)
			if err != nil {
				return nil, err
			}

			if holder, ok := res.(sein.ResponseHolder); ok {
				resp := sein.OK[any](holder.ResponseBody()).WithStatus(holder.StatusCode())
				for k, vv := range holder.ResponseHeaders() {
					for _, v := range vv {
						resp = resp.WithHeader(k, v)
					}
				}

				for _, h := range headers {
					if len(resp.ResponseHeaders()[h.key]) == 0 {
						resp = resp.WithHeader(h.key, h.val)
					}
				}

				return resp, nil
			}

			resp := sein.OK[any](res)
			if resp.StatusCode() == 0 {
				resp = resp.WithStatus(http.StatusOK)
			}

			for _, h := range headers {
				resp = resp.WithHeader(h.key, h.val)
			}

			return resp, nil
		}
	}
}
