// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jwt_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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

func TestJWT_Ed25519_RSA_ECDSA(t *testing.T) {
	// 1. Ed25519
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	edToken, err := jwt.Sign(jwt.MapClaims{"sub": "ed_user"}, edPriv, jwt.EdDSA)
	require.NoError(t, err)

	token, err := jwt.Parse(edToken, edPub)
	require.NoError(t, err)
	assert.Equal(t, "ed_user", token.Claims["sub"])

	// 2. RSA 2048
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rsaPub := &rsaPriv.PublicKey

	for _, alg := range []string{jwt.RS256, jwt.RS384, jwt.RS512} {
		rsaToken, err := jwt.Sign(jwt.MapClaims{"sub": "rsa_user", "alg": alg}, rsaPriv, alg)
		require.NoError(t, err)

		tok, err := jwt.Parse(rsaToken, rsaPub)
		require.NoError(t, err)
		assert.Equal(t, "rsa_user", tok.Claims["sub"])
	}

	// 3. ECDSA P-256
	ecdsaPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecdsaPub := &ecdsaPriv.PublicKey

	esToken, err := jwt.Sign(jwt.MapClaims{"sub": "es_user"}, ecdsaPriv, jwt.ES256)
	require.NoError(t, err)

	tokES, err := jwt.Parse(esToken, ecdsaPub)
	require.NoError(t, err)
	assert.Equal(t, "es_user", tokES.Claims["sub"])

	// 4. HMAC HS384 / HS512
	hmacKey := []byte("secret-key-for-sha512-test-must-be-long")
	for _, alg := range []string{jwt.HS384, jwt.HS512} {
		tokStr, err := jwt.Sign(jwt.MapClaims{"sub": "hmac_user"}, hmacKey, alg)
		require.NoError(t, err)

		parsed, err := jwt.Parse(tokStr, hmacKey)
		require.NoError(t, err)
		assert.Equal(t, "hmac_user", parsed.Claims["sub"])
	}

	// 5. Invalid signature rejection
	badKey := []byte("wrong-key")
	_, err = jwt.Parse(edToken, badKey)
	assert.Error(t, err)
}

func TestJWT_CookieLookup_And_Filter(t *testing.T) {
	key := []byte("jwt-cookie-key-12345")
	validToken, err := jwt.Sign(jwt.MapClaims{"sub": "cookie_user"}, key, jwt.HS256)
	require.NoError(t, err)

	app := sein.New()
	app.Use(jwt.New(
		jwt.WithKey(key),
		jwt.WithTokenLookup("cookie:auth_token"),
		jwt.WithFilter(func(req *sein.Request) bool {
			return req.Path() == "/public"
		}),
	))

	app.Get("/public", func(req *sein.Request) (string, error) {
		return "public-data", nil
	})

	app.Get("/private", func(req *sein.Request) (string, error) {
		claims, _ := jwt.Claims(req)
		return "user:" + claims["sub"].(string), nil
	})

	// Public bypasses JWT
	recPub := httptest.NewRecorder()
	reqPub := httptest.NewRequest(http.MethodGet, "/public", nil)
	app.ServeHTTP(recPub, reqPub)
	assert.Equal(t, http.StatusOK, recPub.Code)

	// Private without cookie -> 401
	recPriv := httptest.NewRecorder()
	reqPriv := httptest.NewRequest(http.MethodGet, "/private", nil)
	app.ServeHTTP(recPriv, reqPriv)
	assert.Equal(t, http.StatusUnauthorized, recPriv.Code)

	// Private with valid cookie -> 200
	recPrivOK := httptest.NewRecorder()
	reqPrivOK := httptest.NewRequest(http.MethodGet, "/private", nil)
	reqPrivOK.AddCookie(&http.Cookie{Name: "auth_token", Value: validToken})
	app.ServeHTTP(recPrivOK, reqPrivOK)
	assert.Equal(t, http.StatusOK, recPrivOK.Code)
	assert.Contains(t, recPrivOK.Body.String(), "user:cookie_user")
}
