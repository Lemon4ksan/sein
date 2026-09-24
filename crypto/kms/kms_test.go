// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kms_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lemon4ksan/sein/crypto/kms"
)

func TestMockKMS(t *testing.T) {
	ctx := context.Background()
	client := kms.NewMockClient()

	plaintext := []byte("secret-dek-payload-12345")
	keyID := "test-key-01"

	ciphertext, err := client.Encrypt(ctx, keyID, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := client.Decrypt(ctx, keyID, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted plaintext mismatch: %s != %s", decrypted, plaintext)
	}

	// Tampered ciphertext
	corruptCT := append([]byte(nil), ciphertext...)
	corruptCT[len(corruptCT)-1] ^= 0xFF
	_, err = client.Decrypt(ctx, keyID, corruptCT)
	if err == nil {
		t.Fatal("expected decryption failure for corrupt ciphertext")
	}

	// Named key isolation
	client2 := kms.NewMockClient()
	_, err = client2.Decrypt(ctx, keyID, ciphertext)
	if err == nil {
		t.Fatal("expected decryption failure using different client/key")
	}
}

func TestParseURI(t *testing.T) {
	tests := []struct {
		uri          string
		wantProvider string
		wantPath     string
		wantErr      bool
	}{
		{
			uri:          "vault://transit/keys/my-app-kek",
			wantProvider: "vault",
			wantPath:     "my-app-kek",
		},
		{
			uri:          "mock://local-test",
			wantProvider: "mock",
			wantPath:     "local-test",
		},
		{
			uri:          "custom://corp-hsm/key-42",
			wantProvider: "custom",
			wantPath:     "corp-hsm/key-42",
		},
		{
			uri:     "",
			wantErr: true,
		},
		{
			uri:     "unknown-without-scheme",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		parsed, err := kms.ParseURI(tc.uri)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("expected error for URI '%s', got nil", tc.uri)
			}
			continue
		}

		if err != nil {
			t.Fatalf("unexpected error for URI '%s': %v", tc.uri, err)
		}
		if parsed.Provider != tc.wantProvider {
			t.Errorf("URI '%s': got provider %s, want %s", tc.uri, parsed.Provider, tc.wantProvider)
		}
		if parsed.KeyPath != tc.wantPath {
			t.Errorf("URI '%s': got path %s, want %s", tc.uri, parsed.KeyPath, tc.wantPath)
		}
	}
}

func TestRouter(t *testing.T) {
	ctx := context.Background()
	router := kms.NewRouter()
	mock := kms.NewMockClient()

	router.Register("mock", mock)

	plaintext := []byte("router-test-secret")
	ct, err := router.Encrypt(ctx, "mock://test-key", plaintext)
	if err != nil {
		t.Fatalf("router.Encrypt failed: %v", err)
	}

	pt, err := router.Decrypt(ctx, "mock://test-key", ct)
	if err != nil {
		t.Fatalf("router.Decrypt failed: %v", err)
	}
	if !bytes.Equal(pt, plaintext) {
		t.Fatalf("mismatch: %s != %s", pt, plaintext)
	}

	// Unregistered provider
	_, err = router.Encrypt(ctx, "unregistered://test", plaintext)
	if !errors.Is(err, kms.ErrUnsupportedProvider) {
		t.Fatalf("expected ErrUnsupportedProvider, got %v", err)
	}
}

func TestVaultClient_MockServer(t *testing.T) {
	ctx := context.Background()
	secret := []byte("vault-secret-payload")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "my-vault-token" {
			http.Error(w, "forbidden", 403)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/v1/transit/encrypt/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]string{
					"ciphertext": "vault:v1:dGVzdC1jaXBoZXJ0ZXh0",
				},
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/v1/transit/decrypt/") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]string{
					"plaintext": base64.StdEncoding.EncodeToString(secret),
				},
			})
			return
		}

		http.Error(w, "not found", 404)
	}))
	defer ts.Close()

	client := kms.NewVaultClient(kms.VaultConfig{
		Address: ts.URL,
		Token:   "my-vault-token",
	})

	keyPath := "vault://transit/keys/my-key"
	ct, err := client.Encrypt(ctx, keyPath, secret)
	if err != nil {
		t.Fatalf("vault encrypt: %v", err)
	}
	if string(ct) != "vault:v1:dGVzdC1jaXBoZXJ0ZXh0" {
		t.Fatalf("unexpected vault ct: %s", string(ct))
	}

	pt, err := client.Decrypt(ctx, keyPath, ct)
	if err != nil {
		t.Fatalf("vault decrypt: %v", err)
	}
	if !bytes.Equal(pt, secret) {
		t.Fatalf("unexpected vault pt: %s", string(pt))
	}
}
