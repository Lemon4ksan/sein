// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package paseto_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/x/paseto"
)

func TestPASETO_PublicAndLocal(t *testing.T) {
	// 1. Asymmetric v4.public test
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	claimsPublic := paseto.MapClaims{
		"sub": "user_ed25519",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	}

	tokenPublic, err := paseto.SignPublic(claimsPublic, privKey)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenPublic)

	verifiedPublic, err := paseto.VerifyPublic(tokenPublic, pubKey)
	require.NoError(t, err)
	assert.Equal(t, "user_ed25519", verifiedPublic["sub"])

	// 2. Symmetric v4.local test
	symKey := sha256.Sum256([]byte("super-secret-paseto-local-key-32b"))
	claimsLocal := paseto.MapClaims{
		"sub": "user_local_encrypted",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	}

	tokenLocal, err := paseto.EncryptLocal(claimsLocal, symKey)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenLocal)

	decryptedLocal, err := paseto.DecryptLocal(tokenLocal, symKey)
	require.NoError(t, err)
	assert.Equal(t, "user_local_encrypted", decryptedLocal["sub"])

	// 3. Middleware test
	app := sein.New()
	app.Use(paseto.New(
		paseto.WithPublicKey(pubKey),
		paseto.WithSymmetricKey(symKey),
	))

	app.Get("/me", func(req *sein.Request) (string, error) {
		claims, ok := paseto.Claims(req)
		if !ok {
			return "", sein.ErrUnauthorized("claims not found")
		}

		return "User: " + claims["sub"].(string), nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// Call with v4.public token -> 200 OK
	req1, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/me", nil)
	req1.Header.Set("Authorization", "Bearer "+tokenPublic)
	resp1, err := client.Do(req1)
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)
	body1, _ := io.ReadAll(resp1.Body)
	assert.Equal(t, "User: user_ed25519", string(body1))

	// Call with v4.local token -> 200 OK
	req2, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/me", nil)
	req2.Header.Set("Authorization", "Bearer "+tokenLocal)
	resp2, err := client.Do(req2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	body2, _ := io.ReadAll(resp2.Body)
	assert.Equal(t, "User: user_local_encrypted", string(body2))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
