// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package idempotency provides HTTP request deduplication and response caching
// complying with the IETF Idempotency-Key specification (RFC 9457).
package idempotency

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/timekit"

	"github.com/lemon4ksan/sein"
)

// HeaderIdempotencyKey is the canonical IETF idempotency header name.
const HeaderIdempotencyKey = "Idempotency-Key"

// CachedResponse stores the serialized HTTP response status, headers, and body for idempotent replay.
type CachedResponse struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body"`
}

// Storage is the pluggable storage engine for idempotency locks and cached responses.
type Storage interface {
	// Get retrieves a cached response by key. Returns false if not found or expired.
	Get(ctx context.Context, key string) ([]byte, bool, error)
	// Set stores a cached response with the given TTL.
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
	// Lock attempts to acquire an exclusive lock for in-flight execution. Returns true if acquired.
	Lock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	// Unlock releases an in-flight lock.
	Unlock(ctx context.Context, key string) error
}

type memoryEntry struct {
	data      []byte
	expiresAt int64
	inFlight  bool
}

type memoryStorage struct {
	mu      sync.RWMutex
	entries map[string]memoryEntry
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{
		entries: make(map[string]memoryEntry),
	}
}

func (m *memoryStorage) Get(_ context.Context, key string) ([]byte, bool, error) {
	now := timekit.CoarseUnix()

	m.mu.RLock()
	entry, ok := m.entries[key]
	m.mu.RUnlock()

	if !ok || entry.inFlight || entry.expiresAt < now {
		return nil, false, nil
	}

	return entry.data, true, nil
}

func (m *memoryStorage) Set(_ context.Context, key string, val []byte, ttl time.Duration) error {
	expiresAt := timekit.CoarseUnix() + int64(ttl.Seconds())

	m.mu.Lock()
	m.entries[key] = memoryEntry{
		data:      val,
		expiresAt: expiresAt,
		inFlight:  false,
	}
	m.mu.Unlock()

	return nil
}

func (m *memoryStorage) Lock(_ context.Context, key string, ttl time.Duration) (bool, error) {
	now := timekit.CoarseUnix()
	expiresAt := now + int64(ttl.Seconds())

	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, exists := m.entries[key]; exists {
		if entry.expiresAt >= now {
			return false, nil
		}
	}

	m.entries[key] = memoryEntry{
		expiresAt: expiresAt,
		inFlight:  true,
	}

	return true, nil
}

func (m *memoryStorage) Unlock(_ context.Context, key string) error {
	m.mu.Lock()
	if entry, exists := m.entries[key]; exists && entry.inFlight {
		delete(m.entries, key)
	}
	m.mu.Unlock()

	return nil
}

// Config configures the Idempotency middleware.
type Config struct {
	// Header is the idempotency header key. Default is "Idempotency-Key".
	Header string
	// Lifetime is the duration cached responses remain valid. Default is 24 hours.
	Lifetime time.Duration
	// LockTimeout is the duration an in-flight request lock is held. Default is 30 seconds.
	LockTimeout time.Duration
	// Storage is the persistence backend. Default is thread-safe in-memory store.
	Storage Storage
	// KeyGenerator is an optional custom key extraction function.
	KeyGenerator func(req *sein.Request) string
}

// Option configures Idempotency settings.
type Option func(*Config)

// WithHeader sets the idempotency header name.
func WithHeader(name string) Option {
	return func(c *Config) {
		c.Header = name
	}
}

// WithLifetime sets the cache TTL for completed idempotent responses.
func WithLifetime(d time.Duration) Option {
	return func(c *Config) {
		c.Lifetime = d
	}
}

// WithLockTimeout sets the in-flight lock TTL.
func WithLockTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.LockTimeout = d
	}
}

// WithStorage sets a custom storage engine.
func WithStorage(s Storage) Option {
	return func(c *Config) {
		c.Storage = s
	}
}

// WithKeyGenerator sets a custom key generator.
func WithKeyGenerator(fn func(req *sein.Request) string) Option {
	return func(c *Config) {
		c.KeyGenerator = fn
	}
}

// New creates an Idempotency middleware complying with RFC 9457.
// Mutating requests bearing an Idempotency-Key are safely deduplicated,
// preventing duplicate processing while replaying identical cached responses.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		Header:      HeaderIdempotencyKey,
		Lifetime:    24 * time.Hour,
		LockTimeout: 30 * time.Second,
		Storage:     newMemoryStorage(),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			// Only apply to mutating methods or requests with explicit idempotency key
			var key string
			if cfg.KeyGenerator != nil {
				key = cfg.KeyGenerator(req)
			} else {
				key = strings.TrimSpace(req.Header(cfg.Header))
			}

			if key == "" {
				return next(req)
			}

			ctx := req.Context()

			// 1. Check if response is already cached
			cachedBytes, found, err := cfg.Storage.Get(ctx, key)
			if err == nil && found && len(cachedBytes) > 0 {
				var cached CachedResponse
				if jsonErr := json.Unmarshal(cachedBytes, &cached); jsonErr == nil {
					resp := sein.OK[any](cached.Body)
					if cached.Status != 0 {
						resp = resp.WithStatus(cached.Status)
					}

					for k, vv := range cached.Headers {
						for _, v := range vv {
							resp = resp.WithHeader(k, v)
						}
					}

					resp = resp.WithHeader("Idempotent-Replayed", "true")

					return resp, nil
				}
			}

			// 2. Acquire lock for in-flight execution
			locked, err := cfg.Storage.Lock(ctx, key, cfg.LockTimeout)
			if err != nil || !locked {
				return nil, sein.ErrConflict("a request with this idempotency key is currently in progress")
			}

			res, err := next(req)
			if err != nil {
				_ = cfg.Storage.Unlock(ctx, key)
				return nil, err
			}

			// 3. Serialize and cache response
			var (
				status   = http.StatusOK
				headers  = make(map[string][]string)
				rawBytes []byte
			)

			if holder, ok := res.(sein.ResponseHolder); ok {
				status = holder.StatusCode()
				for k, vv := range holder.ResponseHeaders() {
					headers[k] = vv
				}

				body := holder.ResponseBody()
				switch b := body.(type) {
				case nil:
					rawBytes = nil
				case []byte:
					rawBytes = b
				case string:
					rawBytes = []byte(b)
				default:
					rawBytes, _ = json.Marshal(b)
					if len(headers[header.ContentType]) == 0 {
						headers[header.ContentType] = []string{header.MIMEApplicationJSONCharsetUTF8}
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
					headers[header.ContentType] = []string{header.MIMEApplicationJSONCharsetUTF8}
				}
			}

			cached := CachedResponse{
				Status:  status,
				Headers: headers,
				Body:    rawBytes,
			}

			if serialized, jsonErr := json.Marshal(cached); jsonErr == nil {
				_ = cfg.Storage.Set(ctx, key, serialized, cfg.Lifetime)
			}

			return res, nil
		}
	}
}
