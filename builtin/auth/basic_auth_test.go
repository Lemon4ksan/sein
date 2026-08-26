// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/auth"
)

func TestBasicAuth_SuccessAndFailure(t *testing.T) {
	s := sein.New()
	s.Use(auth.BasicAuth(map[string]string{
		"admin": "secret_pass_123",
	}, "AdminArea"))

	s.Get("/secret", func(ctx context.Context) (string, error) {
		return "top_secret_data", nil
	})

	// 1. Missing Authorization header -> 401
	req1 := httptest.NewRequest("GET", "/secret", nil)
	w1 := httptest.NewRecorder()
	s.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusUnauthorized, w1.Code)
	assert.Equal(t, "Basic realm=\"AdminArea\"", w1.Header().Get(header.WWWAuthenticate))

	// 2. Wrong Password -> 401
	req2 := httptest.NewRequest("GET", "/secret", nil)
	req2.Header.Set(header.Authorization, "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:wrong_pass")))
	w2 := httptest.NewRecorder()
	s.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusUnauthorized, w2.Code)

	// 3. Valid Credentials -> 200 OK
	req3 := httptest.NewRequest("GET", "/secret", nil)
	req3.Header.Set(header.Authorization, "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret_pass_123")))
	w3 := httptest.NewRecorder()
	s.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusOK, w3.Code)
	assert.Equal(t, "top_secret_data", w3.Body.String())
}
