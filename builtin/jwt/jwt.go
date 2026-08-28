// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package jwt provides high-performance, RFC 7519 compliant JSON Web Token authentication middleware
// supporting HS256/384/512, RS256/384/512, ES256/384/512, and EdDSA (Ed25519) signatures.
package jwt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/lemon4ksan/foundation/codec/json"

	"github.com/lemon4ksan/sein"
)

// Supported cryptographic signature algorithms.
const (
	HS256 = "HS256"
	HS384 = "HS384"
	HS512 = "HS512"
	RS256 = "RS256"
	RS384 = "RS384"
	RS512 = "RS512"
	ES256 = "ES256"
	ES384 = "ES384"
	ES512 = "ES512"
	EdDSA = "EdDSA"
)

var (
	// ErrMissingToken is returned when no token is present in request headers or cookies.
	ErrMissingToken = errors.New("jwt: missing authorization token")
	// ErrInvalidToken is returned when the token format is invalid.
	ErrInvalidToken = errors.New("jwt: malformed token structure")
	// ErrInvalidSignature is returned when cryptographic signature verification fails.
	ErrInvalidSignature = errors.New("jwt: invalid token signature")
	// ErrTokenExpired is returned when the token 'exp' claim has lapsed.
	ErrTokenExpired = errors.New("jwt: token has expired")
	// ErrTokenNotYetValid is returned when the token 'nbf' claim is in the future.
	ErrTokenNotYetValid = errors.New("jwt: token is not valid yet")
	// ErrUnsupportedAlg is returned when the token algorithm is not supported.
	ErrUnsupportedAlg = errors.New("jwt: unsupported signing algorithm")
)

// MapClaims represents unstructured JWT claims.
type MapClaims map[string]any

// Token represents a parsed and verified JWT token.
type Token struct {
	Raw       string
	Header    map[string]any
	Claims    MapClaims
	Algorithm string
	Valid     bool
}

// Config configures the JWT middleware.
type Config struct {
	// Key is the secret or public key used for verification.
	Key any
	// SigningMethod is the expected algorithm (e.g. HS256, RS256).
	SigningMethod string
	// Issuer validates the 'iss' claim if non-empty.
	Issuer string
	// Audience validates the 'aud' claim if non-empty.
	Audience string
	// TokenLookup is the extraction strategy (e.g. "header:Authorization", "cookie:jwt"). Default is "header:Authorization".
	TokenLookup string
	// AuthScheme is the authorization header scheme prefix. Default is "Bearer".
	AuthScheme string
	// ErrorHandler is invoked when validation fails. Default returns HTTP 401 Unauthorized.
	ErrorHandler func(req *sein.Request, err error) (any, error)
	// Filter skips JWT validation when returning true.
	Filter func(req *sein.Request) bool
}

// Option configures JWT settings.
type Option func(*Config)

// WithKey sets the verification key (e.g. []byte for HMAC, *rsa.PublicKey, *ecdsa.PublicKey, ed25519.PublicKey).
func WithKey(key any) Option {
	return func(c *Config) {
		c.Key = key
	}
}

// WithSigningMethod sets the expected cryptographic algorithm.
func WithSigningMethod(method string) Option {
	return func(c *Config) {
		c.SigningMethod = method
	}
}

// WithIssuer sets required issuer claim validation.
func WithIssuer(iss string) Option {
	return func(c *Config) {
		c.Issuer = iss
	}
}

// WithAudience sets required audience claim validation.
func WithAudience(aud string) Option {
	return func(c *Config) {
		c.Audience = aud
	}
}

// WithTokenLookup sets the token extraction source.
func WithTokenLookup(lookup string) Option {
	return func(c *Config) {
		c.TokenLookup = lookup
	}
}

// WithErrorHandler overrides the rejection error handler.
func WithErrorHandler(handler func(req *sein.Request, err error) (any, error)) Option {
	return func(c *Config) {
		c.ErrorHandler = handler
	}
}

// WithFilter configures a bypass predicate.
func WithFilter(fn func(req *sein.Request) bool) Option {
	return func(c *Config) {
		c.Filter = fn
	}
}

// Sign creates and signs a new JWT token using the specified algorithm and private/secret key.
func Sign(claims any, key any, alg string) (string, error) {
	headerData, err := json.Marshal(map[string]string{
		"typ": "JWT",
		"alg": alg,
	})
	if err != nil {
		return "", err
	}

	claimsData, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerData)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsData)
	signingInput := headerB64 + "." + claimsB64

	sig, err := signPayload([]byte(signingInput), key, alg)
	if err != nil {
		return "", err
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func signPayload(input []byte, key any, alg string) ([]byte, error) {
	switch alg {
	case HS256, HS384, HS512:
		secret, ok := key.([]byte)
		if !ok {
			if s, isStr := key.(string); isStr {
				secret = []byte(s)
			} else {
				return nil, errors.New("jwt: HMAC key must be []byte or string")
			}
		}

		var h hash.Hash
		switch alg {
		case HS256:
			h = hmac.New(sha256.New, secret)
		case HS384:
			h = hmac.New(sha512.New384, secret)
		case HS512:
			h = hmac.New(sha512.New, secret)
		}

		h.Write(input)

		return h.Sum(nil), nil

	case RS256, RS384, RS512:
		privKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("jwt: RSA signing requires *rsa.PrivateKey")
		}

		var (
			hashType crypto.Hash
			hashed   []byte
		)

		switch alg {
		case RS256:
			hashType = crypto.SHA256
			sum := sha256.Sum256(input)
			hashed = sum[:]
		case RS384:
			hashType = crypto.SHA384
			sum := sha512.Sum384(input)
			hashed = sum[:]
		case RS512:
			hashType = crypto.SHA512
			sum := sha512.Sum512(input)
			hashed = sum[:]
		}

		return rsa.SignPKCS1v15(rand.Reader, privKey, hashType, hashed)

	case ES256, ES384, ES512:
		privKey, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("jwt: ECDSA signing requires *ecdsa.PrivateKey")
		}

		var (
			hashed []byte
			keyLen int
		)
		switch alg {
		case ES256:
			sum := sha256.Sum256(input)
			hashed = sum[:]
			keyLen = 32
		case ES384:
			sum := sha512.Sum384(input)
			hashed = sum[:]
			keyLen = 48
		case ES512:
			sum := sha512.Sum512(input)
			hashed = sum[:]
			keyLen = 66
		}

		r, s, err := ecdsa.Sign(rand.Reader, privKey, hashed)
		if err != nil {
			return nil, err
		}

		sig := make([]byte, keyLen*2)
		r.FillBytes(sig[:keyLen])
		s.FillBytes(sig[keyLen:])
		return sig, nil

	case EdDSA:
		privKey, ok := key.(ed25519.PrivateKey)
		if !ok {
			return nil, errors.New("jwt: EdDSA signing requires ed25519.PrivateKey")
		}

		return ed25519.Sign(privKey, input), nil

	default:
		return nil, ErrUnsupportedAlg
	}
}

// Parse verifies and extracts claims from a raw JWT token string.
func Parse(tokenStr string, key any) (*Token, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var headerMap map[string]any
	if err := json.Unmarshal(headerBytes, &headerMap); err != nil {
		return nil, ErrInvalidToken
	}

	alg, _ := headerMap["alg"].(string)
	if alg == "" {
		return nil, ErrUnsupportedAlg
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims MapClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}

	signingInput := []byte(parts[0] + "." + parts[1])
	if err := verifySignature(signingInput, sigBytes, key, alg); err != nil {
		return nil, err
	}

	// Validate standard claims
	now := time.Now().Unix()
	if expVal, ok := claims["exp"]; ok {
		var exp int64
		switch v := expVal.(type) {
		case float64:
			exp = int64(v)
		case int64:
			exp = v
		}

		if exp > 0 && now > exp {
			return nil, ErrTokenExpired
		}
	}

	if nbfVal, ok := claims["nbf"]; ok {
		var nbf int64
		switch v := nbfVal.(type) {
		case float64:
			nbf = int64(v)
		case int64:
			nbf = v
		}

		if nbf > 0 && now < nbf {
			return nil, ErrTokenNotYetValid
		}
	}

	return &Token{
		Raw:       tokenStr,
		Header:    headerMap,
		Claims:    claims,
		Algorithm: alg,
		Valid:     true,
	}, nil
}

func verifySignature(input, sig []byte, key any, alg string) error {
	switch alg {
	case HS256, HS384, HS512:
		secret, ok := key.([]byte)
		if !ok {
			if s, isStr := key.(string); isStr {
				secret = []byte(s)
			} else {
				return errors.New("jwt: HMAC verification key must be []byte or string")
			}
		}

		var h hash.Hash
		switch alg {
		case HS256:
			h = hmac.New(sha256.New, secret)
		case HS384:
			h = hmac.New(sha512.New384, secret)
		case HS512:
			h = hmac.New(sha512.New, secret)
		}

		h.Write(input)
		expectedSig := h.Sum(nil)

		if subtle.ConstantTimeCompare(sig, expectedSig) != 1 {
			return ErrInvalidSignature
		}

		return nil

	case RS256, RS384, RS512:
		pubKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return errors.New("jwt: RSA verification requires *rsa.PublicKey")
		}

		var (
			hashType crypto.Hash
			hashed   []byte
		)

		switch alg {
		case RS256:
			hashType = crypto.SHA256
			sum := sha256.Sum256(input)
			hashed = sum[:]
		case RS384:
			hashType = crypto.SHA384
			sum := sha512.Sum384(input)
			hashed = sum[:]
		case RS512:
			hashType = crypto.SHA512
			sum := sha512.Sum512(input)
			hashed = sum[:]
		}

		if err := rsa.VerifyPKCS1v15(pubKey, hashType, hashed, sig); err != nil {
			return ErrInvalidSignature
		}

		return nil

	case ES256, ES384, ES512:
		pubKey, ok := key.(*ecdsa.PublicKey)
		if !ok {
			return errors.New("jwt: ECDSA verification requires *ecdsa.PublicKey")
		}

		var hashed []byte
		switch alg {
		case ES256:
			sum := sha256.Sum256(input)
			hashed = sum[:]
		case ES384:
			sum := sha512.Sum384(input)
			hashed = sum[:]
		case ES512:
			sum := sha512.Sum512(input)
			hashed = sum[:]
		}

		r := new(big.Int).SetBytes(sig[:len(sig)/2])
		s := new(big.Int).SetBytes(sig[len(sig)/2:])

		if !ecdsa.Verify(pubKey, hashed, r, s) {
			return ErrInvalidSignature
		}

		return nil

	case EdDSA:
		pubKey, ok := key.(ed25519.PublicKey)
		if !ok {
			return errors.New("jwt: EdDSA verification requires ed25519.PublicKey")
		}

		if !ed25519.Verify(pubKey, input, sig) {
			return ErrInvalidSignature
		}

		return nil

	default:
		return ErrUnsupportedAlg
	}
}

// Claims extracts the parsed JWT claims from the request context.
func Claims(req *sein.Request) (MapClaims, bool) {
	return sein.Get[MapClaims](req)
}

// New creates an RFC 7519 JWT authentication middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		TokenLookup: "header:Authorization",
		AuthScheme:  "Bearer",
		ErrorHandler: func(_ *sein.Request, err error) (any, error) {
			return nil, sein.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			if cfg.Filter != nil && cfg.Filter(req) {
				return next(req)
			}

			// Extract token string
			var tokenStr string
			if headerKey, ok := strings.CutPrefix(cfg.TokenLookup, "header:"); ok {
				authHdr := req.Header(headerKey)
				if authHdr != "" {
					prefix := cfg.AuthScheme + " "
					if after, ok := strings.CutPrefix(authHdr, prefix); ok {
						tokenStr = after
					} else {
						tokenStr = authHdr
					}
				}
			} else if cookieName, ok := strings.CutPrefix(cfg.TokenLookup, "cookie:"); ok {
				tokenStr, _ = req.Cookie(cookieName)
			}

			if tokenStr == "" {
				return cfg.ErrorHandler(req, ErrMissingToken)
			}

			token, err := Parse(tokenStr, cfg.Key)
			if err != nil {
				return cfg.ErrorHandler(req, err)
			}

			if cfg.Issuer != "" {
				if iss, _ := token.Claims["iss"].(string); iss != cfg.Issuer {
					return cfg.ErrorHandler(req, fmt.Errorf("jwt: invalid issuer: expected %s, got %s", cfg.Issuer, iss))
				}
			}

			// Save claims into flat typed context storage (0 B/op)
			sein.Set(req, token.Claims)
			sein.Set(req, *token)

			return next(req)
		}
	}
}
