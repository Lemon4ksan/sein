// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package revision provides application version, Git commit hash, and build timestamp
// metadata injection middleware and /version diagnostic endpoint.
package revision

import (
	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// Info holds application release and build metadata.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildTime string `json:"build_time,omitempty"`
}

// Config configures the Revision middleware.
type Config struct {
	// Info contains release and build metadata.
	Info Info
	// HeaderName is the HTTP response header for version injection. Default is "X-App-Version".
	HeaderName string
	// CommitHeaderName is the HTTP response header for commit SHA. Default is "X-Git-Commit".
	CommitHeaderName string
	// Path is the endpoint path for version JSON exposition. Default is "/version".
	Path string
}

// Option configures Revision settings.
type Option func(*Config)

// WithVersion sets the application release version.
func WithVersion(v string) Option {
	return func(c *Config) {
		c.Info.Version = v
	}
}

// WithCommit sets the Git commit hash.
func WithCommit(commit string) Option {
	return func(c *Config) {
		c.Info.Commit = commit
	}
}

// WithBuildTime sets the build timestamp string.
func WithBuildTime(t string) Option {
	return func(c *Config) {
		c.Info.BuildTime = t
	}
}

// WithPath sets the version endpoint route path.
func WithPath(path string) Option {
	return func(c *Config) {
		c.Path = path
	}
}

// New creates a revision metadata middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		Info:             Info{Version: "1.0.0"},
		HeaderName:       "X-App-Version",
		CommitHeaderName: "X-Git-Commit",
		Path:             "/version",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			res, err := next(req)

			if holder, ok := res.(sein.ResponseHolder); ok {
				resp := sein.OK[any](holder.ResponseBody()).
					WithStatus(holder.StatusCode()).
					WithHeader(cfg.HeaderName, cfg.Info.Version)

				if cfg.Info.Commit != "" {
					resp = resp.WithHeader(cfg.CommitHeaderName, cfg.Info.Commit)
				}

				for k, vv := range holder.ResponseHeaders() {
					for _, v := range vv {
						resp = resp.WithHeader(k, v)
					}
				}

				return resp, err
			}

			resp := sein.OK[any](res).
				WithHeader(cfg.HeaderName, cfg.Info.Version)

			if cfg.Info.Commit != "" {
				resp = resp.WithHeader(cfg.CommitHeaderName, cfg.Info.Commit)
			}

			return resp, err
		}
	}
}

// Register attaches the revision middleware and the /version JSON endpoint to the server.
func Register(app *sein.Server, opts ...Option) {
	cfg := Config{
		Info:             Info{Version: "1.0.0"},
		HeaderName:       "X-App-Version",
		CommitHeaderName: "X-Git-Commit",
		Path:             "/version",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	app.Use(New(opts...))

	app.Get(cfg.Path, func(_ *sein.Request) (any, error) {
		data, err := json.Marshal(cfg.Info)
		if err != nil {
			return nil, err
		}

		return sein.OK[any](data).
			WithHeader(header.ContentType, header.MIMEApplicationJSONCharsetUTF8), nil
	})
}
