// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package compress provides an ultra-fast, zero-allocation HTTP response compression middleware
// supporting Zstandard (zstd), Brotli (br), and Gzip.
package compress

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/internal/compress"
	"github.com/lemon4ksan/sein/internal/compress/zstd"
)

// Config configures the HTTP response compression middleware.
type Config struct {
	// MinLength is the minimum response payload size in bytes before compression is activated. Default is 512 bytes.
	MinLength int
	// ZstdLevel is the Zstandard compression level (SpeedFastest, SpeedDefault, etc.).
	ZstdLevel zstd.EncoderLevel
	// BrotliLevel is the Brotli compression quality (0-11). Default is 6.
	BrotliLevel int
	// GzipLevel is the Gzip compression level (1-9). Default is 6.
	GzipLevel int
	// MaxDecompressedSize is the maximum allowed size in bytes for incoming compressed request bodies. Default is 32 MB.
	MaxDecompressedSize int64
}

// Option configures compression settings.
type Option func(*Config)

// WithMinLength sets the minimum byte threshold for activating compression.
func WithMinLength(minLen int) Option {
	return func(c *Config) {
		c.MinLength = minLen
	}
}

// WithZstdLevel sets the Zstandard compression speed level.
func WithZstdLevel(level zstd.EncoderLevel) Option {
	return func(c *Config) {
		c.ZstdLevel = level
	}
}

// WithBrotliLevel sets the Brotli compression level (0 = fastest, 6 = default HTTP, 11 = best).
func WithBrotliLevel(level int) Option {
	return func(c *Config) {
		c.BrotliLevel = level
	}
}

// WithMaxDecompressedSize sets the maximum allowed size in bytes for decompressed request bodies.
func WithMaxDecompressedSize(size int64) Option {
	return func(c *Config) {
		c.MaxDecompressedSize = size
	}
}

// New creates a new response compression middleware supporting Zstd, Brotli, and Gzip.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		MinLength:   512,
		ZstdLevel:   zstd.SpeedDefault,
		BrotliLevel: compress.BrotliDefaultCompression,
		GzipLevel:   gzip.DefaultCompression,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			res, err := next(req)
			if err != nil {
				return nil, err
			}

			acceptEncoding := req.Header(header.AcceptEncoding)
			if acceptEncoding == "" {
				return res, nil
			}

			// Extract raw response bytes and existing headers
			var (
				rawBytes        []byte
				contentType     string
				status          int
				existingHeaders http.Header
			)

			if holder, ok := res.(sein.ResponseHolder); ok {
				status = holder.StatusCode()

				existingHeaders = holder.ResponseHeaders()
				if existingHeaders != nil {
					contentType = existingHeaders.Get(header.ContentType)
				}

				body := holder.ResponseBody()
				switch b := body.(type) {
				case nil:
					return res, nil
				case []byte:
					rawBytes = b
				case string:
					rawBytes = []byte(b)
				default:
					rawBytes, _ = json.Marshal(b)

					if contentType == "" {
						contentType = header.MIMEApplicationJSONCharsetUTF8
					}
				}
			} else {
				switch v := res.(type) {
				case []byte:
					rawBytes = v
				case string:
					rawBytes = []byte(v)
				default:
					rawBytes, _ = json.Marshal(v)
					contentType = header.MIMEApplicationJSONCharsetUTF8
				}
			}

			if len(rawBytes) < cfg.MinLength {
				return res, nil
			}

			buildResponse := func(compressed []byte, encoding string) (any, error) {
				resp := sein.OK[any](compressed).
					WithHeader(header.ContentEncoding, encoding).
					WithHeader(header.Vary, header.AcceptEncoding)
				if status != 0 {
					resp = resp.WithStatus(status)
				}

				if contentType != "" {
					resp = resp.WithHeader(header.ContentType, contentType)
				}

				for k, vv := range existingHeaders {
					if !strings.EqualFold(k, header.ContentEncoding) && !strings.EqualFold(k, header.Vary) &&
						!strings.EqualFold(k, header.ContentLength) {
						for _, v := range vv {
							resp = resp.WithHeader(k, v)
						}
					}
				}

				return resp, nil
			}

			// 1. Zstandard (highest priority: fastest compression & decompression throughput)
			if strings.Contains(acceptEncoding, "zstd") {
				compressed, compErr := compress.CompressZstd(rawBytes, cfg.ZstdLevel)
				if compErr == nil && len(compressed) < len(rawBytes) {
					return buildResponse(compressed, "zstd")
				}
			}

			// 2. Brotli (optimal compression ratio for JSON and web text)
			if strings.Contains(acceptEncoding, "br") {
				compressed, compErr := compress.CompressBrotli(rawBytes, cfg.BrotliLevel)
				if compErr == nil && len(compressed) < len(rawBytes) {
					return buildResponse(compressed, "br")
				}
			}

			// 3. Gzip (universal legacy HTTP compatibility)
			if strings.Contains(acceptEncoding, "gzip") {
				compressed, compErr := compress.CompressGzip(rawBytes, cfg.GzipLevel)
				if compErr == nil && len(compressed) < len(rawBytes) {
					return buildResponse(compressed, "gzip")
				}
			}

			return res, nil
		}
	}
}

// RequestDecompressor creates a middleware that automatically inspects and decompresses
// incoming compressed request bodies (Content-Encoding: zstd, br, gzip, deflate) with strict
// decompression bomb protection capped at [Config.MaxDecompressedSize].
func RequestDecompressor(opts ...Option) sein.Middleware {
	cfg := Config{
		MaxDecompressedSize: 32 << 20, // 32 MB default
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			contentEncoding := req.Header(header.ContentEncoding)
			if contentEncoding == "" || strings.EqualFold(contentEncoding, "identity") {
				return next(req)
			}

			body := req.RawBody()
			if len(body) == 0 {
				return next(req)
			}

			decompressed, err := compress.DecompressLimit(contentEncoding, body, cfg.MaxDecompressedSize)
			if err != nil {
				if errors.Is(err, compress.ErrDecompressionLimit) {
					return nil, sein.ErrRequestEntityTooLarge("decompression payload limit exceeded")
				}

				return nil, sein.ErrBadRequest("failed to decompress request payload", err)
			}

			req.SetBody(decompressed)
			req.DelHeader(header.ContentEncoding)

			return next(req)
		}
	}
}
