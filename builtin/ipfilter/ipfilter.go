// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ipfilter provides zero-allocation IP address and CIDR subnet access control list (ACL)
// firewall middleware, supporting granular allow/block list enforcement.
package ipfilter

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/lemon4ksan/sein"
)

type ipMatcher struct {
	exactIPs map[string]struct{}
	cidrs    []*net.IPNet
}

func newIPMatcher(rules []string) (*ipMatcher, error) {
	matcher := &ipMatcher{
		exactIPs: make(map[string]struct{}),
		cidrs:    make([]*net.IPNet, 0, len(rules)),
	}

	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}

		if strings.Contains(rule, "/") {
			_, ipNet, err := net.ParseCIDR(rule)
			if err != nil {
				return nil, fmt.Errorf("ipfilter: invalid CIDR notation %q: %w", rule, err)
			}

			matcher.cidrs = append(matcher.cidrs, ipNet)
		} else {
			ip := net.ParseIP(rule)
			if ip == nil {
				return nil, fmt.Errorf("ipfilter: invalid IP address %q", rule)
			}

			matcher.exactIPs[ip.String()] = struct{}{}
		}
	}

	return matcher, nil
}

func (m *ipMatcher) Matches(ip net.IP) bool {
	if ip == nil {
		return false
	}

	if _, ok := m.exactIPs[ip.String()]; ok {
		return true
	}

	for _, cidr := range m.cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
}

// Config configures the IP Filter middleware.
type Config struct {
	// Allow is a list of allowed IP addresses or CIDR subnets. If non-empty, only matching IPs are permitted.
	Allow []string
	// Block is a list of blocked IP addresses or CIDR subnets. Matching IPs are rejected.
	Block []string
	// ErrorHandler is invoked when an unauthorized IP is rejected. Default returns HTTP 403 Forbidden.
	ErrorHandler func(req *sein.Request) (any, error)
}

// Option configures IP Filter settings.
type Option func(*Config)

// WithAllow adds allowed IPs or CIDRs.
func WithAllow(rules ...string) Option {
	return func(c *Config) {
		c.Allow = append(c.Allow, rules...)
	}
}

// WithBlock adds blocked IPs or CIDRs.
func WithBlock(rules ...string) Option {
	return func(c *Config) {
		c.Block = append(c.Block, rules...)
	}
}

// WithErrorHandler overrides the rejection error handler.
func WithErrorHandler(handler func(req *sein.Request) (any, error)) Option {
	return func(c *Config) {
		c.ErrorHandler = handler
	}
}

// New creates an IP Filter ACL middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		ErrorHandler: func(_ *sein.Request) (any, error) {
			return nil, sein.NewHTTPError(http.StatusForbidden, "IP_FORBIDDEN", "access denied: client IP address is not authorized")
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	var (
		allowMatcher *ipMatcher
		blockMatcher *ipMatcher
	)

	if len(cfg.Allow) > 0 {
		allowMatcher, _ = newIPMatcher(cfg.Allow)
	}

	if len(cfg.Block) > 0 {
		blockMatcher, _ = newIPMatcher(cfg.Block)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			clientIPStr := req.ClientIP()
			ip := net.ParseIP(clientIPStr)
			if ip == nil {
				return cfg.ErrorHandler(req)
			}

			// 1. Check blacklist
			if blockMatcher != nil && blockMatcher.Matches(ip) {
				return cfg.ErrorHandler(req)
			}

			// 2. Check whitelist
			if allowMatcher != nil && !allowMatcher.Matches(ip) {
				return cfg.ErrorHandler(req)
			}

			return next(req)
		}
	}
}
