// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rewrite provides URL path and query rewriting middleware for backward compatibility,
// legacy URL translation, and clean API routing.
package rewrite

import (
	"regexp"
	"strings"

	"github.com/lemon4ksan/sein"
)

// RegexRule represents a compiled regular expression rewrite rule.
type RegexRule struct {
	Regex       *regexp.Regexp
	Replacement string
}

// Config configures the Rewrite middleware.
type Config struct {
	// Rules defines exact or wildcard path rewrite mappings (e.g. "/old": "/new", "/api/v1/*": "/v1/$1").
	Rules map[string]string
	// RegexRules defines regexp rewrite rules.
	RegexRules []RegexRule
}

// Option configures Rewrite settings.
type Option func(*Config)

// WithRules adds a map of path rewrite rules.
func WithRules(rules map[string]string) Option {
	return func(c *Config) {
		if c.Rules == nil {
			c.Rules = make(map[string]string, len(rules))
		}

		for k, v := range rules {
			c.Rules[k] = v
		}
	}
}

// WithRule adds a single path rewrite rule.
func WithRule(from, to string) Option {
	return func(c *Config) {
		if c.Rules == nil {
			c.Rules = make(map[string]string)
		}

		c.Rules[from] = to
	}
}

// WithRegexRule compiles and adds a regular expression rewrite rule.
func WithRegexRule(pattern, replacement string) Option {
	re := regexp.MustCompile(pattern)

	return func(c *Config) {
		c.RegexRules = append(c.RegexRules, RegexRule{
			Regex:       re,
			Replacement: replacement,
		})
	}
}

// New creates a URL path rewrite middleware.
// Incoming request paths matching configured exact, wildcard, or regex rules
// are rewritten before reaching subsequent middlewares and route handlers.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		Rules: make(map[string]string),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			path := req.Path()

			// 1. Exact match rewrite
			if target, ok := cfg.Rules[path]; ok {
				req.SetPath(target)
				return next(req)
			}

			// 2. Wildcard match (e.g. "/users/*" -> "/v2/users/$1")
			for from, to := range cfg.Rules {
				if strings.HasSuffix(from, "/*") {
					prefix := strings.TrimSuffix(from, "/*")
					if strings.HasPrefix(path, prefix+"/") || path == prefix {
						rest := strings.TrimPrefix(path, prefix)
						if strings.HasSuffix(to, "/$1") {
							newPath := strings.TrimSuffix(to, "/$1") + rest
							req.SetPath(newPath)
							return next(req)
						}

						newPath := to + rest
						req.SetPath(newPath)

						return next(req)
					}
				}
			}

			// 3. Regexp match
			for _, rule := range cfg.RegexRules {
				if rule.Regex.MatchString(path) {
					newPath := rule.Regex.ReplaceAllString(path, rule.Replacement)
					req.SetPath(newPath)

					return next(req)
				}
			}

			return next(req)
		}
	}
}
