// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package encryptcookie provides transparent, authenticated AES-256-GCM cookie encryption
// and decryption middleware for sein HTTP pipelines.
package encryptcookie

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

var (
	// ErrInvalidCiphertext is returned when cookie decryption fails or ciphertext is tampered.
	ErrInvalidCiphertext = errors.New("encryptcookie: invalid ciphertext or authentication tag")
)

// Config configures the EncryptCookie middleware.
type Config struct {
	// Key is the 32-byte AES-256 encryption key.
	Key [32]byte
	// Cookies is the list of specific cookie names to encrypt/decrypt. If empty, all cookies are encrypted.
	Cookies []string
	// Except is the list of cookie names excluded from encryption.
	Except []string
}

// Option configures EncryptCookie settings.
type Option func(*Config)

// WithKey sets the secret key string (automatically hashed with SHA-256 into a 32-byte key).
func WithKey(secret string) Option {
	return func(c *Config) {
		c.Key = sha256.Sum256([]byte(secret))
	}
}

// WithRawKey sets the exact 32-byte encryption key.
func WithRawKey(key [32]byte) Option {
	return func(c *Config) {
		c.Key = key
	}
}

// WithCookies specifies targeted cookie names to encrypt.
func WithCookies(names ...string) Option {
	return func(c *Config) {
		c.Cookies = names
	}
}

// WithExcept specifies cookie names that must remain unencrypted.
func WithExcept(names ...string) Option {
	return func(c *Config) {
		c.Except = names
	}
}

// Encrypt encrypts plain text using AES-256-GCM and encodes it to URL-safe base64.
func Encrypt(value string, key [32]byte) (string, error) {
	if value == "" {
		return "", nil
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	sealed := gcm.Seal(nonce, nonce, []byte(value), nil)

	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt decodes and decrypts an AES-256-GCM ciphertext from URL-safe base64.
func Decrypt(encoded string, key [32]byte) (string, error) {
	if encoded == "" {
		return "", nil
	}

	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidCiphertext
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrInvalidCiphertext
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]

	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrInvalidCiphertext
	}

	return string(plain), nil
}

// New creates an EncryptCookie middleware.
// Incoming cookies matching the configuration are transparently decrypted before handler execution,
// and outgoing Set-Cookie responses are automatically encrypted with AES-256-GCM.
func New(opts ...Option) sein.Middleware {
	cfg := Config{}
	for _, opt := range opts {
		opt(&cfg)
	}

	shouldEncrypt := func(name string) bool {
		if slices.Contains(cfg.Except, name) {
			return false
		}

		if len(cfg.Cookies) > 0 {
			return slices.Contains(cfg.Cookies, name)
		}

		return true
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			// 1. Inbound: Decrypt request Cookie header
			cookieHeader := req.Header(header.Cookie)
			if cookieHeader != "" {
				parts := strings.Split(cookieHeader, ";")
				var modified []string

				for _, part := range parts {
					part = strings.TrimSpace(part)
					k, v, found := strings.Cut(part, "=")

					if found && shouldEncrypt(k) {
						if decrypted, err := Decrypt(v, cfg.Key); err == nil {
							modified = append(modified, k+"="+decrypted)
							continue
						}
					}

					modified = append(modified, part)
				}

				req.SetHeader(header.Cookie, strings.Join(modified, "; "))
			}

			// 2. Execute downstream handler
			res, err := next(req)
			if err != nil {
				return nil, err
			}

			// 3. Outbound: Encrypt response cookies
			if holder, ok := res.(sein.ResponseHolder); ok {
				cookies := holder.ResponseCookies()
				if len(cookies) > 0 {
					for _, c := range cookies {
						if c != nil && shouldEncrypt(c.Name) {
							if encrypted, err := Encrypt(c.Value, cfg.Key); err == nil {
								c.Value = encrypted
							}
						}
					}
				}
			}

			return res, nil
		}
	}
}
