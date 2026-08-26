// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package etag provides RFC 7232 conditional requests middleware, computing HTTP ETags
// and short-circuiting unchanged responses with HTTP 304 Not Modified.
package etag

import (
	"fmt"
	"hash/crc32"
	"net/http"
	"strings"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// Config configures the ETag middleware.
type Config struct {
	// Weak generates a weak ETag (prefixed with W/). Default is false (strong ETag).
	Weak bool
}

// Option configures ETag settings.
type Option func(*Config)

// WithWeak configures whether weak ETags are generated.
func WithWeak(weak bool) Option {
	return func(c *Config) {
		c.Weak = weak
	}
}

// New creates an ETag middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			res, err := next(req)
			if err != nil {
				return nil, err
			}

			// Only evaluate ETags for successful 2xx responses
			var (
				bodyBytes []byte
				status    = http.StatusOK
				headers   map[string][]string
			)

			if holder, ok := res.(sein.ResponseHolder); ok {
				status = holder.StatusCode()
				headers = holder.ResponseHeaders()

				raw := holder.ResponseBody()
				switch b := raw.(type) {
				case []byte:
					bodyBytes = b
				case string:
					bodyBytes = []byte(b)
				default:
					if data, mErr := json.Marshal(b); mErr == nil {
						bodyBytes = data
					}
				}
			} else {
				switch b := res.(type) {
				case []byte:
					bodyBytes = b
				case string:
					bodyBytes = []byte(b)
				default:
					if data, mErr := json.Marshal(b); mErr == nil {
						bodyBytes = data
					}
				}
			}

			if status < 200 || status >= 300 || len(bodyBytes) == 0 {
				return res, nil
			}

			checksum := crc32.ChecksumIEEE(bodyBytes)
			tagVal := fmt.Sprintf("\"%d-%08x\"", len(bodyBytes), checksum)
			if cfg.Weak {
				tagVal = "W/" + tagVal
			}

			// Check If-None-Match
			if clientETag := req.Header(header.IfNoneMatch); clientETag != "" {
				clientETag = strings.TrimSpace(clientETag)
				if clientETag == "*" || clientETag == tagVal || strings.TrimPrefix(clientETag, "W/") == strings.TrimPrefix(tagVal, "W/") {
					respHeaders := make(map[string][]string, len(headers)+1)
					for k, vv := range headers {
						respHeaders[k] = vv
					}

					respHeaders[header.ETag] = []string{tagVal}

					return sein.StatusWith[any](http.StatusNotModified, nil, respHeaders), nil
				}
			}

			if holder, ok := res.(sein.ResponseHolder); ok {
				resp := sein.OK[any](holder.ResponseBody()).
					WithStatus(holder.StatusCode()).
					WithHeader(header.ETag, tagVal)

				for k, vv := range holder.ResponseHeaders() {
					for _, v := range vv {
						resp = resp.WithHeader(k, v)
					}
				}

				return resp, nil
			}

			return sein.OK[any](res).WithHeader(header.ETag, tagVal), nil
		}
	}
}
