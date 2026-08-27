// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"

	"github.com/lemon4ksan/sein"
)

func TestVersionMatrix_RoutingAndFilters(t *testing.T) {
	app := sein.New()

	v := app.Versioned("1", "2", "3")

	// 1. Universal route on all versions
	v.Get("/health", func(ctx context.Context) (string, error) {
		return "ok", nil
	})

	// 2. Route available Since v2 (v2, v3)
	v.Since("2").Get("/users", func(ctx context.Context) (string, error) {
		return "users list", nil
	})

	// 3. Route available Only Until v2 (v1, v2)
	v.Until("2").Get("/legacy", func(ctx context.Context) (string, error) {
		return "legacy data", nil
	})

	// 4. Route Only on v3
	v.Only("3").Post("/bots/:id/disconnect", func(ctx context.Context, id uint64) (string, error) {
		return "disconnected", nil
	})

	// 5. Route Between v2 and v3
	v.Between("2", "3").Get("/pricing", func(ctx context.Context) (string, error) {
		return "pricing", nil
	})

	// Test requests
	tests := []struct {
		method   string
		path     string
		expected int
	}{
		// Health on all
		{"GET", "/v1/health", http.StatusOK},
		{"GET", "/v2/health", http.StatusOK},
		{"GET", "/v3/health", http.StatusOK},

		// Users since v2
		{"GET", "/v1/users", http.StatusNotFound},
		{"GET", "/v2/users", http.StatusOK},
		{"GET", "/v3/users", http.StatusOK},

		// Legacy until v2
		{"GET", "/v1/legacy", http.StatusOK},
		{"GET", "/v2/legacy", http.StatusOK},
		{"GET", "/v3/legacy", http.StatusNotFound},

		// Bot disconnect only v3
		{"POST", "/v1/bots/123/disconnect", http.StatusNotFound},
		{"POST", "/v2/bots/123/disconnect", http.StatusNotFound},
		{"POST", "/v3/bots/123/disconnect", http.StatusOK},

		// Pricing between 2 and 3
		{"GET", "/v1/pricing", http.StatusNotFound},
		{"GET", "/v2/pricing", http.StatusOK},
		{"GET", "/v3/pricing", http.StatusOK},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		assert.Equal(t, tc.expected, rec.Code, tc.method+" "+tc.path)
	}
}

func TestVersionMatrix_GuardsAndGroups(t *testing.T) {
	app := sein.New()

	authMiddleware := func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			if req.Header("X-Secret") != "valid" {
				return nil, sein.ErrUnauthorized("missing secret")
			}
			return next(req)
		}
	}

	v := app.Versioned("v2", "v3")

	v.Guard(authMiddleware).Do(func(g *sein.VersionGroup) {
		users := g.Group("/users")
		users.Get("/:id", func(ctx context.Context, id uint64) (uint64, error) {
			return id, nil
		})
	})

	// Unauthorized
	req1 := httptest.NewRequest("GET", "/v2/users/42", nil)
	rec1 := httptest.NewRecorder()
	app.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusUnauthorized, rec1.Code)

	// Authorized on v2
	req2 := httptest.NewRequest("GET", "/v2/users/42", nil)
	req2.Header.Set("X-Secret", "valid")
	rec2 := httptest.NewRecorder()
	app.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)

	// Authorized on v3
	req3 := httptest.NewRequest("GET", "/v3/users/42", nil)
	req3.Header.Set("X-Secret", "valid")
	rec3 := httptest.NewRecorder()
	app.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusOK, rec3.Code)
}
