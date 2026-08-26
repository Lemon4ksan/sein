// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// BasicAuthConfig defines the configuration for the HTTP Basic Authentication middleware.
type BasicAuthConfig struct {
	// Accounts is a username-to-password lookup map.
	Accounts map[string]string

	// Validator is an optional custom verification function (e.g. database lookup).
	// If provided, Accounts is ignored.
	Validator func(username, password string) bool

	// Realm is the realm name displayed in the client's authentication challenge dialog.
	// Default is "Restricted".
	Realm string
}

// BasicAuth returns an HTTP Basic Authentication middleware verifying credentials against accounts.
func BasicAuth(accounts map[string]string, realm ...string) sein.Middleware {
	r := "Restricted"
	if len(realm) > 0 && realm[0] != "" {
		r = realm[0]
	}

	return NewBasicAuth(BasicAuthConfig{
		Accounts: accounts,
		Realm:    r,
	})
}

// NewBasicAuth creates an HTTP Basic Authentication middleware with custom configuration.
func NewBasicAuth(cfg BasicAuthConfig) sein.Middleware {
	if cfg.Realm == "" {
		cfg.Realm = "Restricted"
	}

	challengeHeader := "Basic realm=" + strconv.Quote(cfg.Realm)

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			authHdr := req.Header(header.Authorization)
			if authHdr == "" || !strings.HasPrefix(authHdr, "Basic ") {
				return unauthorizedResponse(challengeHeader)
			}

			payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHdr, "Basic "))
			if err != nil {
				return unauthorizedResponse(challengeHeader)
			}

			user, pass, found := strings.Cut(string(payload), ":")
			if !found {
				return unauthorizedResponse(challengeHeader)
			}

			// Validate credentials
			if cfg.Validator != nil {
				if !cfg.Validator(user, pass) {
					return unauthorizedResponse(challengeHeader)
				}
			} else if cfg.Accounts != nil {
				expectedPass, exists := cfg.Accounts[user]
				var passMatch int
				if exists {
					passMatch = subtle.ConstantTimeCompare([]byte(pass), []byte(expectedPass))
				} else {
					// Dummy constant time comparison against fixed string to prevent timing attack enumeration
					_ = subtle.ConstantTimeCompare([]byte(pass), []byte("dummy-constant-time-pass-protection"))
				}

				if !exists || passMatch != 1 {
					return unauthorizedResponse(challengeHeader)
				}
			} else {
				return unauthorizedResponse(challengeHeader)
			}

			return next(req)
		}
	}
}

func unauthorizedResponse(challenge string) (any, error) {
	headers := make(http.Header)
	headers.Set(header.WWWAuthenticate, challenge)

	return sein.StatusWith(http.StatusUnauthorized, map[string]string{
		"error": "unauthorized",
	}, headers), nil
}
