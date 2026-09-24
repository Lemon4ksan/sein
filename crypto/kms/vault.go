// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kms

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// VaultConfig configures access to HashiCorp Vault.
type VaultConfig struct {
	Address    string
	Token      string
	Namespace  string // Optional Vault Enterprise namespace
	HTTPClient *http.Client
}

// VaultClient implements KMSClient for HashiCorp Vault Transit Secret Engine.
type VaultClient struct {
	address    string
	token      string
	namespace  string
	httpClient *http.Client
}

// NewVaultClient creates a Vault Transit client with explicit configuration.
func NewVaultClient(cfg VaultConfig) *VaultClient {
	addr := strings.TrimSpace(cfg.Address)
	if addr == "" {
		addr = "http://127.0.0.1:8200"
	}
	addr = strings.TrimSuffix(addr, "/")

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &VaultClient{
		address:    addr,
		token:      cfg.Token,
		namespace:  cfg.Namespace,
		httpClient: httpClient,
	}
}

// NewVaultClientFromEnv creates a Vault Transit client loading configuration from standard environment variables:
// VAULT_ADDR, VAULT_TOKEN, and VAULT_NAMESPACE.
func NewVaultClientFromEnv() *VaultClient {
	return NewVaultClient(VaultConfig{
		Address:   os.Getenv("VAULT_ADDR"),
		Token:     os.Getenv("VAULT_TOKEN"),
		Namespace: os.Getenv("VAULT_NAMESPACE"),
	})
}

// Encrypt wraps plaintext using HashiCorp Vault Transit Secret Engine.
func (v *VaultClient) Encrypt(ctx context.Context, keyPath string, plaintext []byte) ([]byte, error) {
	cleanPath := strings.TrimPrefix(keyPath, "vault://")
	cleanPath = strings.TrimPrefix(cleanPath, "transit/keys/")
	url := fmt.Sprintf("%s/v1/transit/encrypt/%s", v.address, cleanPath)

	reqBody := map[string]string{
		"plaintext": base64.StdEncoding.EncodeToString(plaintext),
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("vault: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", v.token)
	req.Header.Set("Content-Type", "application/json")
	if v.namespace != "" {
		req.Header.Set("X-Vault-Namespace", v.namespace)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("vault transit error %d: %s", resp.StatusCode, string(respBytes))
	}

	var vaultResp struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &vaultResp); err != nil {
		return nil, fmt.Errorf("vault: decode response: %w", err)
	}

	return []byte(vaultResp.Data.Ciphertext), nil
}

// Decrypt unwraps ciphertext using HashiCorp Vault Transit Secret Engine.
func (v *VaultClient) Decrypt(ctx context.Context, keyPath string, ciphertext []byte) ([]byte, error) {
	cleanPath := strings.TrimPrefix(keyPath, "vault://")
	cleanPath = strings.TrimPrefix(cleanPath, "transit/keys/")
	url := fmt.Sprintf("%s/v1/transit/decrypt/%s", v.address, cleanPath)

	reqBody := map[string]string{
		"ciphertext": string(ciphertext),
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("vault: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", v.token)
	req.Header.Set("Content-Type", "application/json")
	if v.namespace != "" {
		req.Header.Set("X-Vault-Namespace", v.namespace)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("vault transit error %d: %s", resp.StatusCode, string(respBytes))
	}

	var vaultResp struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &vaultResp); err != nil {
		return nil, fmt.Errorf("vault: decode response: %w", err)
	}

	pt, err := base64.StdEncoding.DecodeString(vaultResp.Data.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("vault: decode plaintext base64: %w", err)
	}
	return pt, nil
}
