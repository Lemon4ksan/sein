// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cors_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/cors"
)

func TestCORS_Default_SimpleRequest(t *testing.T) {
	s := sein.New()
	s.Use(cors.Default())

	s.Get("/data", func(ctx context.Context) (map[string]string, error) {
		return map[string]string{"message": "success"}, nil
	})

	req := httptest.NewRequest("GET", "/data", nil)
	req.Header.Set(header.Origin, "https://frontend.example.com")
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get(header.AccessControlAllowOrigin))
}

func TestCORS_Preflight_WithOptions(t *testing.T) {
	s := sein.New()
	s.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://*.lemon.app", "https://lemon.dev"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           3600,
	}))

	s.PostAction("/api/v1/resource", func(ctx context.Context) (string, error) {
		return "created", nil
	})

	// Preflight OPTIONS request
	req := httptest.NewRequest("OPTIONS", "/api/v1/resource", nil)
	req.Header.Set(header.Origin, "https://dashboard.lemon.app")
	req.Header.Set(header.AccessControlRequestMethod, "POST")
	req.Header.Set(header.AccessControlRequestHeaders, "Authorization, Content-Type")
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "https://dashboard.lemon.app", w.Header().Get(header.AccessControlAllowOrigin))
	assert.Equal(t, "true", w.Header().Get(header.AccessControlAllowCredentials))
	assert.Equal(t, "GET, POST, PUT, DELETE", w.Header().Get(header.AccessControlAllowMethods))
	assert.Equal(t, "Authorization, Content-Type", w.Header().Get(header.AccessControlAllowHeaders))
	assert.Equal(t, "3600", w.Header().Get(header.AccessControlMaxAge))

	// Invalid Origin test
	reqInvalid := httptest.NewRequest("OPTIONS", "/api/v1/resource", nil)
	reqInvalid.Header.Set(header.Origin, "https://malicious.hacker.com")
	reqInvalid.Header.Set(header.AccessControlRequestMethod, "POST")
	wInvalid := httptest.NewRecorder()

	s.ServeHTTP(wInvalid, reqInvalid)

	// Since origin did not match, no CORS headers returned
	assert.Empty(t, wInvalid.Header().Get(header.AccessControlAllowOrigin))
}
