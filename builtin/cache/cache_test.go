// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/cache"
)

func TestCache_HitMissAndExpiration(t *testing.T) {
	var handlerCounter atomic.Int64

	cacheMW, store := cache.New(
		cache.WithExpiration(200 * time.Millisecond),
	)

	app := sein.New()
	app.Use(cacheMW)

	app.Get("/data", func(ctx context.Context) (string, error) {
		_ = handlerCounter.Add(1)
		return "computed-data-v1", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. First call -> MISS, counter = 1
	resp1, err := client.Get("http://" + addr + "/data")
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	assert.Equal(t, "MISS", resp1.Header.Get("X-Cache"))
	body1, _ := io.ReadAll(resp1.Body)
	assert.Equal(t, "computed-data-v1", string(body1))
	assert.Equal(t, int64(1), handlerCounter.Load())

	// 2. Second call -> HIT, counter still = 1
	resp2, err := client.Get("http://" + addr + "/data")
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, "HIT", resp2.Header.Get("X-Cache"))
	body2, _ := io.ReadAll(resp2.Body)
	assert.Equal(t, "computed-data-v1", string(body2))
	assert.Equal(t, int64(1), handlerCounter.Load())

	// 3. Manual deletion from store -> MISS, counter = 2
	store.Delete("GET:/data")
	resp3, err := client.Get("http://" + addr + "/data")
	require.NoError(t, err)
	defer func() { _ = resp3.Body.Close() }()
	assert.Equal(t, "MISS", resp3.Header.Get("X-Cache"))
	assert.Equal(t, int64(2), handlerCounter.Load())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestCache_Tags_NoCache_And_Options(t *testing.T) {
	var callCount atomic.Int64

	cacheMW, store := cache.New(
		cache.WithExpiration(10*time.Minute),
		cache.WithKeyGenerator(func(req *sein.Request) string {
			return req.Path()
		}),
		cache.WithCacheHeader(false),
	)

	app := sein.New()
	app.Use(cacheMW)

	app.Get("/users", func(ctx context.Context) (sein.Response[string], error) {
		_ = callCount.Add(1)
		return sein.OK("users-list"), nil
	})

	app.Post("/users", func(ctx context.Context) (string, error) {
		return "created", nil
	})

	// 1. First GET -> calls handler (callCount = 1), no X-Cache header because CacheHeader=false
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/users", nil)
	app.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, int64(1), callCount.Load())
	assert.Empty(t, rec1.Header().Get("X-Cache"))

	// 2. Second GET with Cache-Control: no-cache -> bypasses cache (callCount = 2)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/users", nil)
	req2.Header.Set("Cache-Control", "no-cache")
	app.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Equal(t, int64(2), callCount.Load())

	// 3. POST method -> not cached
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/users", nil)
	app.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusOK, rec3.Code)

	// 4. InvalidateTags
	store.InvalidateTags("users-tag")

	// Standalone Middleware constructor
	standaloneMW := cache.Middleware()
	assert.NotNil(t, standaloneMW)
}
