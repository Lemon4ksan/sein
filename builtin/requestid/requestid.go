// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package requestid

import (
	"context"
	"encoding/hex"
	"math/rand/v2"
	"net/http"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/timekit"

	"github.com/lemon4ksan/sein"
)

type contextKey struct{}

var (
	requestIDKey = contextKey{}
	counter      uint64
)

// Config defines the configuration for the Request ID middleware.
type Config struct {
	// HeaderName is the HTTP header key used for the Request ID.
	// Default is "X-Request-ID".
	HeaderName string

	// Generator creates a unique identifier string.
	// Default generates a 32-character fast hex ID.
	Generator func() string
}

// DefaultConfig provides default settings for Request ID.
var DefaultConfig = Config{
	HeaderName: header.XRequestID,
	Generator:  generateID,
}

func generateID() string {
	var b [16]byte

	now := timekit.CoarseUnixNano()
	seq := atomic.AddUint64(&counter, 1)

	b[0] = byte(now >> 56)
	b[1] = byte(now >> 48)
	b[2] = byte(now >> 40)
	b[3] = byte(now >> 32)
	b[4] = byte(now >> 24)
	b[5] = byte(now >> 16)
	b[6] = byte(now >> 8)
	b[7] = byte(now)

	b[8] = byte(seq >> 56)
	b[9] = byte(seq >> 48)
	b[10] = byte(seq >> 40)
	b[11] = byte(seq >> 32)
	b[12] = byte(seq >> 24)
	b[13] = byte(seq >> 16)
	b[14] = byte(seq >> 8)
	b[15] = byte(rand.Uint32()) //nolint:gosec // Non-cryptographic random request ID

	var dst [32]byte
	hex.Encode(dst[:], b[:])

	return string(dst[:])
}

// Default returns a Request ID middleware with default configuration.
func Default() sein.Middleware {
	return New(DefaultConfig)
}

// New returns a new Request ID middleware with the given configuration.
func New(config ...Config) sein.Middleware {
	cfg := DefaultConfig
	if len(config) > 0 {
		cfg = config[0]
		if cfg.HeaderName == "" {
			cfg.HeaderName = DefaultConfig.HeaderName
		}

		if cfg.Generator == nil {
			cfg.Generator = DefaultConfig.Generator
		}
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			id := req.Header(cfg.HeaderName)
			if id == "" {
				id = cfg.Generator()
			}

			// Store ID in request context
			ctx := context.WithValue(req.Context(), requestIDKey, id)
			req.SetContext(ctx)

			result, err := next(req)
			if err != nil {
				return nil, err
			}

			return attachHeader(result, cfg.HeaderName, id), nil
		}
	}
}

// FromContext extracts the Request ID from a context.Context.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}

	return ""
}

// Get extracts the Request ID from a sein.Request.
func Get(req *sein.Request) string {
	if req == nil {
		return ""
	}

	return FromContext(req.Context())
}

func attachHeader(result any, headerName, id string) any {
	if holder, ok := result.(sein.ResponseHolder); ok {
		headers := holder.ResponseHeaders()
		if headers == nil {
			headers = make(http.Header)
		}

		headers.Set(headerName, id)

		return sein.StatusWith(holder.StatusCode(), holder.ResponseBody(), headers)
	}

	headers := make(http.Header)
	headers.Set(headerName, id)

	return sein.StatusWith(http.StatusOK, result, headers)
}
