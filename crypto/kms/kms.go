// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package kms provides key management service provider abstractions, client registration,
// and URI-based key resolution.
package kms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	// ErrUnsupportedProvider indicates that no KMS client is registered for the specified URI scheme.
	ErrUnsupportedProvider = errors.New("kms: unsupported key provider")

	// ErrInvalidURI indicates an invalid KMS URI syntax.
	ErrInvalidURI = errors.New("kms: invalid key URI")

	// ErrDecryptionFailed indicates the KMS service could not decrypt the ciphertext.
	ErrDecryptionFailed = errors.New("kms: decryption failed")
)

// Client defines the common interface for Key Management Service envelope encryption providers
// (e.g., HashiCorp Vault Transit, enterprise KMS/HSM, or in-memory mock KMS).
type Client interface {
	// Encrypt encrypts a plaintext DEK or secret using the specified key ID or URI.
	Encrypt(ctx context.Context, keyID string, plaintext []byte) ([]byte, error)

	// Decrypt decrypts a KMS ciphertext blob to recover the plaintext DEK or secret.
	Decrypt(ctx context.Context, keyID string, ciphertext []byte) ([]byte, error)
}

// KeyURI represents a parsed KMS key identifier.
type KeyURI struct {
	Provider string // "vault", "mock", or custom provider name
	KeyPath  string // Key name, transit path, or provider-specific ID
	Raw      string // Original raw URI string
}

// ParseURI parses a key identifier or URI into provider and key path.
// Supported formats:
//   - "vault://<transit-key-path>" -> Provider: "vault", KeyPath: <transit-key-path>
//   - "mock://<key-id>" -> Provider: "mock", KeyPath: <key-id>
//   - "<provider>://<key-path>" -> Provider: "<provider>", KeyPath: "<key-path>"
func ParseURI(rawURI string) (*KeyURI, error) {
	trimmed := strings.TrimSpace(rawURI)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: URI is empty", ErrInvalidURI)
	}

	if before, after, ok := strings.Cut(trimmed, "://"); ok {
		scheme := strings.ToLower(before)
		path := after
		switch scheme {
		case "vault":
			return &KeyURI{Provider: "vault", KeyPath: strings.TrimPrefix(path, "transit/keys/"), Raw: trimmed}, nil
		case "mock":
			return &KeyURI{Provider: "mock", KeyPath: path, Raw: trimmed}, nil
		default:
			return &KeyURI{Provider: scheme, KeyPath: path, Raw: trimmed}, nil
		}
	}

	// Default fallback: if no scheme provided, return error
	return nil, fmt.Errorf("%w: unable to determine provider from '%s'", ErrInvalidURI, rawURI)
}

// Router dispatches KMS operations to registered provider clients based on URI schemes.
type Router struct {
	mu      sync.RWMutex
	clients map[string]Client
}

// NewRouter creates a thread-safe KMS router.
func NewRouter() *Router {
	return &Router{
		clients: make(map[string]Client),
	}
}

// Register registers a KMS client for the specified provider scheme (e.g. "aws", "vault", "mock").
func (r *Router) Register(provider string, client Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[strings.ToLower(provider)] = client
}

// ClientFor resolves the appropriate Client and KeyPath for a given key URI.
func (r *Router) ClientFor(rawURI string) (Client, string, error) {
	parsed, err := ParseURI(rawURI)
	if err != nil {
		return nil, "", err
	}

	r.mu.RLock()
	client, ok := r.clients[parsed.Provider]
	r.mu.RUnlock()

	if !ok {
		return nil, "", fmt.Errorf("%w: '%s'", ErrUnsupportedProvider, parsed.Provider)
	}

	return client, parsed.KeyPath, nil
}

// Encrypt resolves the client from rawURI and executes encryption.
func (r *Router) Encrypt(ctx context.Context, rawURI string, plaintext []byte) ([]byte, error) {
	client, keyPath, err := r.ClientFor(rawURI)
	if err != nil {
		return nil, err
	}
	return client.Encrypt(ctx, keyPath, plaintext)
}

// Decrypt resolves the client from rawURI and executes decryption.
func (r *Router) Decrypt(ctx context.Context, rawURI string, ciphertext []byte) ([]byte, error) {
	client, keyPath, err := r.ClientFor(rawURI)
	if err != nil {
		return nil, err
	}
	return client.Decrypt(ctx, keyPath, ciphertext)
}
