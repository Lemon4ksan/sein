// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cache provides RFC 7234 HTTP response caching middleware for idempotent routes,
// with configurable TTL, thread-safe memory storage, Age headers, and tag-based invalidation.
package cache

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

type cacheEntry struct {
	bodyBytes []byte
	status    int
	headers   map[string][]string
	createdAt time.Time
	expiresAt time.Time
	tags      []string
}

// Store represents the cache storage engine interface.
type Store interface {
	Get(key string) (*cacheEntry, bool)
	Set(key string, entry *cacheEntry)
	Delete(key string)
	InvalidateTags(tags ...string)
}

type memoryStore struct {
	lru     *generic.LRU[string, *cacheEntry]
	mu      sync.RWMutex
	tagKeys map[string]map[string]struct{}
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		lru:     generic.NewLRU[string, *cacheEntry](10000),
		tagKeys: make(map[string]map[string]struct{}),
	}
}

func (s *memoryStore) Get(key string) (*cacheEntry, bool) {
	entry, ok := s.lru.Get(key)
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return entry, true
}

func (s *memoryStore) Set(key string, entry *cacheEntry) {
	s.lru.Put(key, entry)

	if len(entry.tags) > 0 {
		s.mu.Lock()
		for _, tag := range entry.tags {
			if s.tagKeys[tag] == nil {
				s.tagKeys[tag] = make(map[string]struct{})
			}
			s.tagKeys[tag][key] = struct{}{}
		}
		s.mu.Unlock()
	}
}

func (s *memoryStore) Delete(key string) {
	s.lru.Delete(key)
}

func (s *memoryStore) InvalidateTags(tags ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, tag := range tags {
		if keys, ok := s.tagKeys[tag]; ok {
			for key := range keys {
				s.lru.Delete(key)
			}
			delete(s.tagKeys, tag)
		}
	}
}

// Config configures the Cache middleware.
type Config struct {
	// Expiration is the validity duration of cached entries. Default is 1 minute.
	Expiration time.Duration
	// KeyGenerator creates cache keys from incoming requests.
	KeyGenerator func(req *sein.Request) string
	// Store is the underlying cache store implementation.
	Store Store
	// CacheHeader enables X-Cache: HIT / MISS header injection. Default is true.
	CacheHeader bool
}

// Option configures Cache settings.
type Option func(*Config)

// WithExpiration sets the default cache TTL duration.
func WithExpiration(d time.Duration) Option {
	return func(c *Config) {
		c.Expiration = d
	}
}

// WithKeyGenerator overrides the cache key construction function.
func WithKeyGenerator(fn func(req *sein.Request) string) Option {
	return func(c *Config) {
		c.KeyGenerator = fn
	}
}

// WithStore sets a custom cache storage implementation.
func WithStore(store Store) Option {
	return func(c *Config) {
		c.Store = store
	}
}

// WithCacheHeader configures whether X-Cache headers are emitted.
func WithCacheHeader(enabled bool) Option {
	return func(c *Config) {
		c.CacheHeader = enabled
	}
}

// New creates an RFC 7234 HTTP response caching middleware.
// GET and HEAD responses are stored in-memory and replayed with zero handler invocation.
func New(opts ...Option) (sein.Middleware, Store) {
	memStore := newMemoryStore()

	cfg := Config{
		Expiration: 1 * time.Minute,
		Store:      memStore,
		CacheHeader: true,
		KeyGenerator: func(req *sein.Request) string {
			return req.Method() + ":" + req.Path()
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	mw := func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			method := req.Method()
			if method != http.MethodGet && method != http.MethodHead {
				return next(req)
			}

			// Respect Cache-Control: no-cache
			if req.Header(header.CacheControl) == "no-cache" {
				return next(req)
			}

			cacheKey := cfg.KeyGenerator(req)

			// 1. Cache HIT
			if entry, hit := cfg.Store.Get(cacheKey); hit {
				age := int(time.Since(entry.createdAt).Seconds())
				resp := sein.OK[any](entry.bodyBytes).
					WithStatus(entry.status).
					WithHeader(header.Age, fmt.Sprintf("%d", age))

				for k, vv := range entry.headers {
					for _, v := range vv {
						resp = resp.WithHeader(k, v)
					}
				}

				if cfg.CacheHeader {
					resp = resp.WithHeader("X-Cache", "HIT")
				}

				return resp, nil
			}

			// 2. Cache MISS: execute handler
			res, err := next(req)
			if err != nil {
				return nil, err
			}

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

			if status >= 200 && status < 300 && len(bodyBytes) > 0 {
				cfg.Store.Set(cacheKey, &cacheEntry{
					bodyBytes: bodyBytes,
					status:    status,
					headers:   headers,
					createdAt: time.Now(),
					expiresAt: time.Now().Add(cfg.Expiration),
				})
			}

			if holder, ok := res.(sein.ResponseHolder); ok {
				resp := sein.OK[any](holder.ResponseBody()).
					WithStatus(holder.StatusCode())

				for k, vv := range holder.ResponseHeaders() {
					for _, v := range vv {
						resp = resp.WithHeader(k, v)
					}
				}

				if cfg.CacheHeader {
					resp = resp.WithHeader("X-Cache", "MISS")
				}

				return resp, nil
			}

			resp := sein.OK[any](res)
			if cfg.CacheHeader {
				resp = resp.WithHeader("X-Cache", "MISS")
			}

			return resp, nil
		}
	}

	return mw, cfg.Store
}

// Middleware creates a standalone cache middleware discarding the Store return.
func Middleware(opts ...Option) sein.Middleware {
	mw, _ := New(opts...)
	return mw
}
