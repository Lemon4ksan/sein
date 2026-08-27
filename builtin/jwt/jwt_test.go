// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jwt_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/jwt"
)

func TestJWT_LifecycleAndMiddleware(t *testing.T) {
	secretKey := []byte("super-secret-production-signing-key-123")

	// 1. Generate valid token
	tokenStr, err := jwt.Sign(jwt.MapClaims{
		"sub":  "user_12345",
		"role": "admin",
		"exp":  time.Now().Add(1 * time.Hour).Unix(),
		"iss":  "sein-auth",
	}, secretKey, jwt.HS256)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	// 2. Generate expired token
	expiredToken, err := jwt.Sign(jwt.MapClaims{
		"sub": "user_expired",
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
	}, secretKey, jwt.HS256)
	require.NoError(t, err)

	// 3. Setup sein server
	app := sein.New()
	app.Use(jwt.New(
		jwt.WithKey(secretKey),
		jwt.WithIssuer("sein-auth"),
	))

	app.Get("/profile", func(req *sein.Request) (string, error) {
		claims, ok := jwt.Claims(req)
		if !ok {
			return "", sein.ErrUnauthorized("claims not found in context")
		}

		return "Hello " + claims["sub"].(string) + ", role: " + claims["role"].(string), nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 4. Request without token -> 401 Unauthorized
	respNoToken, err := client.Get("http://" + addr + "/profile")
	require.NoError(t, err)
	defer func() { _ = respNoToken.Body.Close() }()
	assert.Equal(t, http.StatusUnauthorized, respNoToken.StatusCode)

	// 5. Request with expired token -> 401 Unauthorized
	reqExpired, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/profile", nil)
	reqExpired.Header.Set("Authorization", "Bearer "+expiredToken)
	respExpired, err := client.Do(reqExpired)
	require.NoError(t, err)
	defer func() { _ = respExpired.Body.Close() }()
	assert.Equal(t, http.StatusUnauthorized, respExpired.StatusCode)

	// 6. Request with valid token -> 200 OK
	reqValid, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/profile", nil)
	reqValid.Header.Set("Authorization", "Bearer "+tokenStr)
	respValid, err := client.Do(reqValid)
	require.NoError(t, err)
	defer func() { _ = respValid.Body.Close() }()
	assert.Equal(t, http.StatusOK, respValid.StatusCode)
	body, _ := io.ReadAll(respValid.Body)
	assert.Equal(t, "Hello user_12345, role: admin", string(body))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
