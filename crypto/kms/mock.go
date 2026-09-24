// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"sync"

	"github.com/lemon4ksan/foundation/silicon/randkit"
)

// MockClient provides an in-memory AES-GCM envelope encryption provider for testing and local development.
type MockClient struct {
	mu         sync.RWMutex
	defaultKey [32]byte
	namedKeys  map[string][32]byte
}

// NewMockClient creates a new in-memory mock KMS client.
// If masterKey is provided, it must be 32 bytes; otherwise, a cryptographic random 32-byte key is generated.
func NewMockClient(masterKey ...[]byte) *MockClient {
	m := &MockClient{
		namedKeys: make(map[string][32]byte),
	}
	if len(masterKey) > 0 && len(masterKey[0]) == 32 {
		copy(m.defaultKey[:], masterKey[0])
	} else {
		randBytes := randkit.MustSecureBytes(32)
		copy(m.defaultKey[:], randBytes)
	}
	return m
}

// SetKey assigns an explicit 32-byte master key for a specific keyID.
func (m *MockClient) SetKey(keyID string, key [32]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.namedKeys[keyID] = key
}

func (m *MockClient) getKey(keyID string) [32]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if k, ok := m.namedKeys[keyID]; ok {
		return k
	}
	return m.defaultKey
}

// Encrypt encrypts plaintext using AES-256-GCM authenticated envelope encryption.
func (m *MockClient) Encrypt(_ context.Context, keyID string, plaintext []byte) ([]byte, error) {
	key := m.getKey(keyID)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("mock kms: cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("mock kms: gcm init: %w", err)
	}

	nonce := randkit.MustSecureBytes(gcm.NonceSize())
	aad := []byte("kms-envelope:" + keyID)
	sealed := gcm.Seal(nonce, nonce, plaintext, aad)
	return sealed, nil
}

// Decrypt recovers plaintext from an envelope ciphertext blob.
func (m *MockClient) Decrypt(_ context.Context, keyID string, ciphertext []byte) ([]byte, error) {
	key := m.getKey(keyID)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("mock kms: cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("mock kms: gcm init: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize+gcm.Overhead() {
		return nil, errors.New("mock kms: invalid ciphertext length")
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	aad := []byte("kms-envelope:" + keyID)
	pt, err := gcm.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDecryptionFailed, err)
	}
	return pt, nil
}
