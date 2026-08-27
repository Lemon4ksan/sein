// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package encryptcookie_test

import (
	"crypto/sha256"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/encryptcookie"
)

func TestEncryptDecrypt_Standalone(t *testing.T) {
	key := sha256.Sum256([]byte("super-secret-production-key-32b"))
	plain := "user-session-token-xyz-12345"

	cipherHex, err := encryptcookie.Encrypt(plain, key)
	require.NoError(t, err)
	assert.NotEmpty(t, cipherHex)
	assert.NotEqual(t, plain, cipherHex)

	decrypted, err := encryptcookie.Decrypt(cipherHex, key)
	require.NoError(t, err)
	assert.Equal(t, plain, decrypted)

	// Tampered ciphertext
	tampered := cipherHex[:len(cipherHex)-2] + "AA"
	_, err = encryptcookie.Decrypt(tampered, key)
	assert.Error(t, err)
}

func TestEncryptCookie_MiddlewareEndToEnd(t *testing.T) {
	const secret = "production-encryption-password-2026"

	app := sein.New()
	app.Use(encryptcookie.New(
		encryptcookie.WithKey(secret),
		encryptcookie.WithCookies("session_id"),
	))

	// Endpoint 1: Sets encrypted cookie
	app.Get("/set-session", func(req *sein.Request) (sein.Response[string], error) {
		return sein.OK("Cookie set").WithCookie(&http.Cookie{
			Name:  "session_id",
			Value: "user-uuid-999-authenticated",
			Path:  "/",
		}), nil
	})

	// Endpoint 2: Reads transparently decrypted cookie
	app.Get("/read-session", func(req *sein.Request) (string, error) {
		val, err := req.Cookie("session_id")
		if err != nil {
			return "", err
		}

		return "Decrypted: " + val, nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Call /set-session to get the encrypted cookie
	resp1, err := client.Get("http://" + addr + "/set-session")
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()

	cookies := resp1.Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session_id" {
			sessionCookie = c
		}
	}

	require.NotNil(t, sessionCookie)
	// Over-the-wire value must NOT be plaintext
	assert.NotEqual(t, "user-uuid-999-authenticated", sessionCookie.Value)

	// 2. Call /read-session sending the encrypted cookie back
	req2, err := http.NewRequest(http.MethodGet, "http://"+addr+"/read-session", nil)
	require.NoError(t, err)
	req2.AddCookie(sessionCookie)

	resp2, err := client.Do(req2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}
