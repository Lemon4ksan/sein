// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package favicon provides zero-allocation favicon serving and log-suppression middleware.
package favicon

import (
	"net/http"
	"os"

	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// DefaultFaviconURL is the standard favicon route path.
const DefaultFaviconURL = "/favicon.ico"

// Config configures the Favicon middleware.
type Config struct {
	// Data contains raw in-memory favicon bytes.
	Data []byte
	// File is the filesystem path to an ICO or PNG icon file loaded into memory.
	File string
	// URL is the route pattern for favicon requests. Default is "/favicon.ico".
	URL string
	// CacheControl sets the Cache-Control response header. Default is "public, max-age=31536000".
	CacheControl string
}

// Option configures Favicon settings.
type Option func(*Config)

// WithData configures in-memory favicon binary payload.
func WithData(data []byte) Option {
	return func(c *Config) {
		c.Data = data
	}
}

// WithFile loads a favicon icon directly from disk on startup.
func WithFile(path string) Option {
	return func(c *Config) {
		c.File = path
	}
}

// WithURL overrides the favicon request URL.
func WithURL(u string) Option {
	return func(c *Config) {
		c.URL = u
	}
}

// WithCacheControl sets the Cache-Control header.
func WithCacheControl(cc string) Option {
	return func(c *Config) {
		c.CacheControl = cc
	}
}

// New creates a favicon serving middleware.
// If an icon file or byte slice is configured, it is served with HTTP 200 and image/x-icon content type.
// Otherwise, it returns an instant HTTP 204 No Content with long-term caching.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		URL:          DefaultFaviconURL,
		CacheControl: "public, max-age=31536000",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	iconData := cfg.Data
	if len(iconData) == 0 && cfg.File != "" {
		// #nosec G304 -- File path is explicitly configured by the application developer
		if data, err := os.ReadFile(cfg.File); err == nil {
			iconData = data
		}
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			if req.Path() == cfg.URL {
				if req.Method() != http.MethodGet && req.Method() != http.MethodHead {
					return sein.StatusWith[any](http.StatusMethodNotAllowed, nil, nil), nil
				}

				if len(iconData) > 0 {
					return sein.OK[any](iconData).
						WithHeader(header.ContentType, "image/x-icon").
						WithHeader(header.CacheControl, cfg.CacheControl), nil
				}

				return sein.StatusWith[any](http.StatusNoContent, nil, map[string][]string{
					header.CacheControl: {cfg.CacheControl},
				}), nil
			}

			return next(req)
		}
	}
}

// Register registers the favicon route directly onto the sein server.
func Register(app *sein.Server, opts ...Option) {
	cfg := Config{
		URL:          DefaultFaviconURL,
		CacheControl: "public, max-age=31536000",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	iconData := cfg.Data
	if len(iconData) == 0 && cfg.File != "" {
		// #nosec G304 -- File path is explicitly configured by the application developer
		if data, err := os.ReadFile(cfg.File); err == nil {
			iconData = data
		}
	}

	app.Get(cfg.URL, func(req *sein.Request) (any, error) {
		if len(iconData) > 0 {
			return sein.OK[any](iconData).
				WithHeader(header.ContentType, "image/x-icon").
				WithHeader(header.CacheControl, cfg.CacheControl), nil
		}

		return sein.StatusWith[any](http.StatusNoContent, nil, map[string][]string{
			header.CacheControl: {cfg.CacheControl},
		}), nil
	})
}
