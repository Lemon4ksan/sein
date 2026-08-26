// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package paseto provides Platform-Agnostic Security Tokens (PASETO v4.public and v4.local)
// authentication and cryptographic verification middleware.
package paseto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lemon4ksan/foundation/codec/json"

	"github.com/lemon4ksan/sein"
)

// PASETO version prefixes.
const (
	V4PublicPrefix = "v4.public."
	V4LocalPrefix  = "v4.local."
)

var (
	// ErrInvalidToken is returned when the token format or version is invalid.
	ErrInvalidToken = errors.New("paseto: invalid token format")
	// ErrInvalidSignature is returned when asymmetric or symmetric verification fails.
	ErrInvalidSignature = errors.New("paseto: invalid token signature")
	// ErrTokenExpired is returned when the token 'exp' claim has lapsed.
	ErrTokenExpired = errors.New("paseto: token has expired")
	// ErrMissingToken is returned when no token was provided in the request.
	ErrMissingToken = errors.New("paseto: missing authorization token")
)

// MapClaims represents unstructured PASETO claims.
type MapClaims map[string]any

// SignPublic signs claims using Ed25519 creating a v4.public PASETO token.
func SignPublic(claims any, privKey ed25519.PrivateKey, footer ...string) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	header := []byte(V4PublicPrefix)
	var f []byte
	if len(footer) > 0 && footer[0] != "" {
		f = []byte(footer[0])
	}

	preAuth := preAuthEncode([][]byte{header, payload, f})
	sig := ed25519.Sign(privKey, preAuth)

	tokenBody := append(payload, sig...)
	tokenB64 := base64.RawURLEncoding.EncodeToString(tokenBody)

	if len(f) > 0 {
		return V4PublicPrefix + tokenB64 + "." + base64.RawURLEncoding.EncodeToString(f), nil
	}

	return V4PublicPrefix + tokenB64, nil
}

// VerifyPublic validates a v4.public PASETO token using an Ed25519 public key.
func VerifyPublic(tokenStr string, pubKey ed25519.PublicKey) (MapClaims, error) {
	if !strings.HasPrefix(tokenStr, V4PublicPrefix) {
		return nil, ErrInvalidToken
	}

	rest := strings.TrimPrefix(tokenStr, V4PublicPrefix)
	parts := strings.Split(rest, ".")

	tokenBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(tokenBytes) < ed25519.SignatureSize {
		return nil, ErrInvalidToken
	}

	var footer []byte
	if len(parts) > 1 {
		footer, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, ErrInvalidToken
		}
	}

	payloadLen := len(tokenBytes) - ed25519.SignatureSize
	payload := tokenBytes[:payloadLen]
	sig := tokenBytes[payloadLen:]

	header := []byte(V4PublicPrefix)
	preAuth := preAuthEncode([][]byte{header, payload, footer})

	if !ed25519.Verify(pubKey, preAuth, sig) {
		return nil, ErrInvalidSignature
	}

	var claims MapClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if expVal, ok := claims["exp"]; ok {
		var exp int64
		switch v := expVal.(type) {
		case float64:
			exp = int64(v)
		case int64:
			exp = v
		}

		if exp > 0 && time.Now().Unix() > exp {
			return nil, ErrTokenExpired
		}
	}

	return claims, nil
}

// EncryptLocal encrypts claims using a 32-byte symmetric key creating a v4.local PASETO token.
func EncryptLocal(claims any, key [32]byte, footer ...string) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
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

	sealed := gcm.Seal(nonce, nonce, payload, []byte(V4LocalPrefix))
	encoded := base64.RawURLEncoding.EncodeToString(sealed)

	if len(footer) > 0 && footer[0] != "" {
		return V4LocalPrefix + encoded + "." + base64.RawURLEncoding.EncodeToString([]byte(footer[0])), nil
	}

	return V4LocalPrefix + encoded, nil
}

// DecryptLocal decrypts and validates a v4.local PASETO token using a 32-byte symmetric key.
func DecryptLocal(tokenStr string, key [32]byte) (MapClaims, error) {
	if !strings.HasPrefix(tokenStr, V4LocalPrefix) {
		return nil, ErrInvalidToken
	}

	rest := strings.TrimPrefix(tokenStr, V4LocalPrefix)
	parts := strings.Split(rest, ".")

	cipherData, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(cipherData) < nonceSize {
		return nil, ErrInvalidToken
	}

	nonce, ciphertext := cipherData[:nonceSize], cipherData[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(V4LocalPrefix))
	if err != nil {
		return nil, ErrInvalidSignature
	}

	var claims MapClaims
	if err := json.Unmarshal(plain, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if expVal, ok := claims["exp"]; ok {
		var exp int64
		switch v := expVal.(type) {
		case float64:
			exp = int64(v)
		case int64:
			exp = v
		}

		if exp > 0 && time.Now().Unix() > exp {
			return nil, ErrTokenExpired
		}
	}

	return claims, nil
}

func preAuthEncode(pieces [][]byte) []byte {
	var buf []byte
	buf = append(buf, uint64ToLE(uint64(len(pieces)))...)

	for _, p := range pieces {
		buf = append(buf, uint64ToLE(uint64(len(p)))...)
		buf = append(buf, p...)
	}

	return buf
}

func uint64ToLE(n uint64) []byte {
	b := make([]byte, 8)
	for i := range 8 {
		b[i] = byte(n >> (i * 8))
	}

	return b
}

// Claims extracts parsed PASETO claims from the request context.
func Claims(req *sein.Request) (MapClaims, bool) {
	return sein.Get[MapClaims](req)
}

// Config configures the PASETO middleware.
type Config struct {
	// PublicKey validates v4.public tokens.
	PublicKey ed25519.PublicKey
	// SymmetricKey validates v4.local tokens.
	SymmetricKey [32]byte
	// Header is the request header containing the token. Default is "Authorization".
	Header string
	// AuthScheme is the authorization header prefix. Default is "Bearer".
	AuthScheme string
	// ErrorHandler is invoked on rejection. Default returns HTTP 401.
	ErrorHandler func(req *sein.Request, err error) (any, error)
}

// Option configures PASETO settings.
type Option func(*Config)

// WithPublicKey sets the Ed25519 public key.
func WithPublicKey(pubKey ed25519.PublicKey) Option {
	return func(c *Config) {
		c.PublicKey = pubKey
	}
}

// WithSymmetricKey sets the 32-byte encryption key.
func WithSymmetricKey(key [32]byte) Option {
	return func(c *Config) {
		c.SymmetricKey = key
	}
}

// WithErrorHandler overrides rejection error handling.
func WithErrorHandler(handler func(req *sein.Request, err error) (any, error)) Option {
	return func(c *Config) {
		c.ErrorHandler = handler
	}
}

// New creates a PASETO authentication middleware supporting v4.public and v4.local tokens.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		Header:     "Authorization",
		AuthScheme: "Bearer",
		ErrorHandler: func(_ *sein.Request, err error) (any, error) {
			return nil, sein.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			authHdr := req.Header(cfg.Header)
			if authHdr == "" {
				return cfg.ErrorHandler(req, ErrMissingToken)
			}

			prefix := cfg.AuthScheme + " "
			tokenStr := authHdr
			if strings.HasPrefix(authHdr, prefix) {
				tokenStr = strings.TrimPrefix(authHdr, prefix)
			}

			var (
				claims MapClaims
				err    error
			)

			switch {
			case strings.HasPrefix(tokenStr, V4PublicPrefix):
				if cfg.PublicKey == nil {
					return cfg.ErrorHandler(req, fmt.Errorf("paseto: public key not configured for v4.public verification"))
				}

				claims, err = VerifyPublic(tokenStr, cfg.PublicKey)

			case strings.HasPrefix(tokenStr, V4LocalPrefix):
				claims, err = DecryptLocal(tokenStr, cfg.SymmetricKey)

			default:
				return cfg.ErrorHandler(req, ErrInvalidToken)
			}

			if err != nil {
				return cfg.ErrorHandler(req, err)
			}

			sein.Set(req, claims)

			return next(req)
		}
	}
}
