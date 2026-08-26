// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package limiter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/limiter"
)

func TestRateLimiter_AllowAndThrottle(t *testing.T) {
	s := sein.New()
	s.Use(limiter.New(limiter.Config{
		Rate:        5,
		Window:      time.Second,
		SendHeaders: true,
	}))

	s.Get("/ping", func(ctx context.Context) (string, error) {
		return "pong", nil
	})

	// Make 5 successful requests
	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest("GET", "/ping", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "5", w.Header().Get("RateLimit-Limit"))
	}

	// 6th request must be throttled with 429
	req := httptest.NewRequest("GET", "/ping", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// Different IP address should still be allowed
	req2 := httptest.NewRequest("GET", "/ping", nil)
	req2.RemoteAddr = "192.168.1.200:54321"
	w2 := httptest.NewRecorder()
	s.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
}

func BenchmarkRateLimiter_ConcurrentAccess(b *testing.B) {
	s := sein.New()
	s.Use(limiter.New(limiter.Config{
		Rate:        1000000,
		Window:      time.Minute,
		SendHeaders: false,
	}))

	s.Get("/bench", func(ctx context.Context) (string, error) {
		return "ok", nil
	})

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("GET", "/bench", nil)
			req.RemoteAddr = "10.0.0.1:8080"
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)
		}
	})
}
