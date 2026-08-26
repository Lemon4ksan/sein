// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package limiter

import (
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/timekit"
	"github.com/lemon4ksan/sein"
)

// Config defines the configuration parameters for the Rate Limiter middleware.
type Config struct {
	// Rate is the maximum number of requests allowed within the Window.
	// Default is 60.
	Rate int

	// Window is the time period for the Rate limit.
	// Default is 1 minute.
	Window time.Duration

	// KeyGenerator extracts an identifier key from incoming requests (e.g. IP address or User ID).
	// Default uses req.RemoteAddr().
	KeyGenerator func(req *sein.Request) string

	// LimitReachedHandler defines the handler invoked when a client exceeds the allowed rate.
	// Default returns HTTP 429 Too Many Requests.
	LimitReachedHandler func(req *sein.Request) (any, error)

	// SendHeaders specifies whether standard RFC 6585 rate limit headers are included in the response.
	// Default is true.
	SendHeaders bool
}

// DefaultConfig provides standard rate limiting defaults (60 requests per minute per IP).
var DefaultConfig = Config{
	Rate:        60,
	Window:      time.Minute,
	SendHeaders: true,
}

type bucket struct {
	count     int64
	resetTime int64 // Unix timestamp in seconds
}

type rateLimiter struct {
	rate         int64
	windowSecs   int64
	keyGen       func(req *sein.Request) string
	limitHandler func(req *sein.Request) (any, error)
	sendHeaders  bool

	shards [64]struct {
		mu      sync.RWMutex
		entries map[string]*bucket
	}
}

func (rl *rateLimiter) getShard(key string) (shardIdx uint64) {
	var hash uint64 = 14695981039346656037
	for i := 0; i < len(key); i++ {
		hash ^= uint64(key[i])
		hash *= 1099511628211
	}

	return hash & 63
}

func (rl *rateLimiter) allow(key string) (allowed bool, remaining, resetInSecs int64) {
	now := timekit.CoarseUnix()
	idx := rl.getShard(key)
	shard := &rl.shards[idx]

	shard.mu.RLock()
	b, exists := shard.entries[key]
	shard.mu.RUnlock()

	if !exists {
		shard.mu.Lock()

		b, exists = shard.entries[key]
		if !exists {
			b = &bucket{
				count:     1,
				resetTime: now + rl.windowSecs,
			}
			shard.entries[key] = b
			shard.mu.Unlock()

			return true, rl.rate - 1, rl.windowSecs
		}

		shard.mu.Unlock()
	}

	// Read reset time
	reset := atomic.LoadInt64(&b.resetTime)
	if now >= reset {
		// Window expired, reset counter
		newReset := now + rl.windowSecs
		if atomic.CompareAndSwapInt64(&b.resetTime, reset, newReset) {
			atomic.StoreInt64(&b.count, 1)
			return true, rl.rate - 1, rl.windowSecs
		}
	}

	count := atomic.AddInt64(&b.count, 1)
	remaining = rl.rate - count
	resetInSecs = max(reset - now, 0)

	if count > rl.rate {
		return false, 0, resetInSecs
	}

	return true, remaining, resetInSecs
}

// Default creates a Rate Limiter middleware with default settings (60 req/min).
func Default() sein.Middleware {
	return New(DefaultConfig)
}

// New creates a Rate Limiter middleware with custom configuration.
func New(config ...Config) sein.Middleware {
	cfg := DefaultConfig
	if len(config) > 0 {
		cfg = config[0]
		if cfg.Rate <= 0 {
			cfg.Rate = DefaultConfig.Rate
		}

		if cfg.Window <= 0 {
			cfg.Window = DefaultConfig.Window
		}
	}

	keyGen := cfg.KeyGenerator
	if keyGen == nil {
		keyGen = func(req *sein.Request) string {
			return req.RemoteAddr()
		}
	}

	limitHandler := cfg.LimitReachedHandler
	if limitHandler == nil {
		limitHandler = func(req *sein.Request) (any, error) {
			return nil, sein.NewHTTPError(
				http.StatusTooManyRequests,
				"TOO_MANY_REQUESTS",
				"rate limit exceeded, please try again later",
			)
		}
	}

	rl := &rateLimiter{
		rate:         int64(cfg.Rate),
		windowSecs:   int64(cfg.Window / time.Second),
		keyGen:       keyGen,
		limitHandler: limitHandler,
		sendHeaders:  cfg.SendHeaders,
	}
	if rl.windowSecs <= 0 {
		rl.windowSecs = 1
	}

	for i := range rl.shards {
		rl.shards[i].entries = make(map[string]*bucket, 128)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			key := rl.keyGen(req)
			allowed, remaining, resetSecs := rl.allow(key)

			if !allowed {
				res, err := rl.limitHandler(req)
				if err != nil {
					return nil, err
				}

				if rl.sendHeaders {
					return attachRateLimitHeaders(res, rl.rate, 0, resetSecs), nil
				}

				return res, nil
			}

			result, err := next(req)
			if err != nil {
				return nil, err
			}

			if rl.sendHeaders {
				return attachRateLimitHeaders(result, rl.rate, remaining, resetSecs), nil
			}

			return result, nil
		}
	}
}

func attachRateLimitHeaders(result any, limit, remaining, resetSecs int64) any {
	rateLimitStr := strconv.FormatInt(limit, 10)
	remainingStr := strconv.FormatInt(remaining, 10)
	resetStr := strconv.FormatInt(resetSecs, 10)

	if holder, ok := result.(sein.ResponseHolder); ok {
		headers := holder.ResponseHeaders()
		if headers == nil {
			headers = make(http.Header)
		}

		headers.Set("RateLimit-Limit", rateLimitStr)
		headers.Set("RateLimit-Remaining", remainingStr)
		headers.Set("RateLimit-Reset", resetStr)

		if remaining == 0 {
			headers.Set("Retry-After", resetStr)
		}

		return sein.StatusWith(holder.StatusCode(), holder.ResponseBody(), headers)
	}

	headers := make(http.Header)
	headers.Set("RateLimit-Limit", rateLimitStr)
	headers.Set("RateLimit-Remaining", remainingStr)
	headers.Set("RateLimit-Reset", resetStr)

	if remaining == 0 {
		headers.Set("Retry-After", resetStr)
	}

	return sein.StatusWith(http.StatusOK, result, headers)
}
