// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package hostauth provides HTTP Host header authorization middleware
// designed to protect against DNS Rebinding, HTTP Host Header Injection,
// and unauthorized virtual host access.
package hostauth

import (
	"net"
	"strings"

	"github.com/lemon4ksan/sein"
)

// Config configures the host authorization middleware.
type Config struct {
	// Hosts defines the list of permitted hostnames and wildcard patterns (e.g. "example.com", "*.example.com", "localhost:*").
	Hosts []string
	// ErrorHandler is the custom error handler invoked when host authorization fails. Default returns HTTP 403 Forbidden.
	ErrorHandler func(req *sein.Request) (any, error)
}

// Option configures host authorization settings.
type Option func(*Config)

// WithHosts sets the allowed host patterns.
func WithHosts(hosts ...string) Option {
	return func(c *Config) {
		c.Hosts = hosts
	}
}

// WithErrorHandler sets a custom error handler for unauthorized hosts.
func WithErrorHandler(handler func(req *sein.Request) (any, error)) Option {
	return func(c *Config) {
		c.ErrorHandler = handler
	}
}

type hostMatcher struct {
	exact      string
	wildcard   string
	port       string
	anyPort    bool
	isWildcard bool
}

// New creates a new Host authorization middleware.
// It verifies incoming request Host headers against the configured whitelist and RFC 1035 length constraints.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		Hosts: []string{"localhost:*", "127.0.0.1:*", "[::1]:*"},
		ErrorHandler: func(_ *sein.Request) (any, error) {
			return nil, sein.ErrForbidden("unauthorized host header")
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	matchers := make([]hostMatcher, 0, len(cfg.Hosts))
	for _, h := range cfg.Hosts {
		h = strings.TrimSpace(strings.ToLower(h))
		if h == "" {
			continue
		}

		m := parsePattern(h)
		matchers = append(matchers, m)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			host := strings.TrimSpace(strings.ToLower(req.Host()))
			if host == "" {
				return cfg.ErrorHandler(req)
			}

			// Validate RFC 1035 length limits (max 253 chars for FQDN, max 63 per label)
			if !isValidRFC1035(host) {
				return cfg.ErrorHandler(req)
			}

			hostname, port := splitHostPort(host)

			for _, m := range matchers {
				if m.matches(hostname, port) {
					return next(req)
				}
			}

			return cfg.ErrorHandler(req)
		}
	}
}

func parsePattern(pattern string) hostMatcher {
	hostname, port := splitHostPort(pattern)

	m := hostMatcher{
		port: port,
	}

	if port == "*" {
		m.anyPort = true
	}

	if strings.HasPrefix(hostname, "*.") {
		m.isWildcard = true
		m.wildcard = strings.TrimPrefix(hostname, "*.")
	} else {
		m.exact = hostname
	}

	return m
}

func (m hostMatcher) matches(hostname, port string) bool {
	// Check port
	if !m.anyPort && m.port != "" && m.port != port {
		return false
	}

	if m.isWildcard {
		if hostname == m.wildcard {
			return true
		}

		if strings.HasSuffix(hostname, "."+m.wildcard) {
			return true
		}

		return false
	}

	return m.exact == hostname
}

func splitHostPort(host string) (hostname, port string) {
	h, p, err := net.SplitHostPort(host)
	if err == nil {
		return h, p
	}

	return host, ""
}

func isValidRFC1035(host string) bool {
	hostname, _ := splitHostPort(host)

	// Trim trailing dot for FQDN
	hostname = strings.TrimSuffix(hostname, ".")

	if len(hostname) == 0 || len(hostname) > 253 {
		return false
	}

	// IPv6 literals (e.g. [::1]) are permitted
	if strings.HasPrefix(hostname, "[") && strings.HasSuffix(hostname, "]") {
		return true
	}

	labels := strings.Split(hostname, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
	}

	return true
}
