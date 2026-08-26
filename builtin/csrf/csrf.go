// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package csrf provides Cross-Site Request Forgery (CSRF) mitigation middleware
// using Double-Submit Cookie validation with constant-time token comparison.
package csrf

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/lemon4ksan/sein"
)

// Default CSRF configuration constants.
const (
	DefaultCookieName = "csrf_token"
	DefaultHeaderName = "X-CSRF-Token"
	DefaultFormField  = "_csrf"
)

var safeMethods = []string{"GET", "HEAD", "OPTIONS", "TRACE"}

// Config configures the CSRF middleware.
type Config struct {
	// CookieName is the name of the CSRF token cookie. Default is "csrf_token".
	CookieName string
	// HeaderName is the HTTP request header where the client supplies the CSRF token. Default is "X-CSRF-Token".
	HeaderName string
	// FormField is the form or query field name for the token. Default is "_csrf".
	FormField string
	// Expiration is the validity duration of the CSRF token cookie. Default is 24 hours.
	Expiration time.Duration
	// CookieSameSite specifies the SameSite policy for the cookie. Default is http.SameSiteLaxMode.
	CookieSameSite http.SameSite
	// CookieSecure sets the Secure attribute on the cookie. Default is false.
	CookieSecure bool
	// CookieHTTPOnly sets the HttpOnly attribute on the cookie. Default is false (allowing frontend JS access).
	CookieHTTPOnly bool
	// CookieDomain sets the Domain attribute on the cookie.
	CookieDomain string
	// CookiePath sets the Path attribute on the cookie. Default is "/".
	CookiePath string
	// ErrorHandler is invoked when CSRF validation fails. Default returns HTTP 403 Forbidden.
	ErrorHandler func(req *sein.Request) (any, error)
}

// Option configures CSRF settings.
type Option func(*Config)

// WithCookieName sets the cookie name.
func WithCookieName(name string) Option {
	return func(c *Config) {
		c.CookieName = name
	}
}

// WithHeaderName sets the header name.
func WithHeaderName(name string) Option {
	return func(c *Config) {
		c.HeaderName = name
	}
}

// WithFormField sets the form field name.
func WithFormField(field string) Option {
	return func(c *Config) {
		c.FormField = field
	}
}

// WithExpiration sets the cookie expiration TTL.
func WithExpiration(d time.Duration) Option {
	return func(c *Config) {
		c.Expiration = d
	}
}

// WithCookieSameSite sets the cookie SameSite mode.
func WithCookieSameSite(mode http.SameSite) Option {
	return func(c *Config) {
		c.CookieSameSite = mode
	}
}

// WithCookieSecure sets whether the cookie is HTTPS-only.
func WithCookieSecure(secure bool) Option {
	return func(c *Config) {
		c.CookieSecure = secure
	}
}

// WithCookieHTTPOnly sets the HttpOnly flag.
func WithCookieHTTPOnly(httpOnly bool) Option {
	return func(c *Config) {
		c.CookieHTTPOnly = httpOnly
	}
}

// WithCookieDomain sets the cookie domain.
func WithCookieDomain(domain string) Option {
	return func(c *Config) {
		c.CookieDomain = domain
	}
}

// WithCookiePath sets the cookie path.
func WithCookiePath(path string) Option {
	return func(c *Config) {
		c.CookiePath = path
	}
}

// WithErrorHandler overrides the CSRF rejection error handler.
func WithErrorHandler(handler func(req *sein.Request) (any, error)) Option {
	return func(c *Config) {
		c.ErrorHandler = handler
	}
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

// New creates a CSRF mitigation middleware.
// Safe methods (GET, HEAD, OPTIONS, TRACE) automatically generate and set the CSRF cookie.
// Mutating methods (POST, PUT, DELETE, PATCH) require a matching token in headers or form fields.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		CookieName:     DefaultCookieName,
		HeaderName:     DefaultHeaderName,
		FormField:      DefaultFormField,
		Expiration:     24 * time.Hour,
		CookieSameSite: http.SameSiteLaxMode,
		CookiePath:     "/",
		ErrorHandler: func(_ *sein.Request) (any, error) {
			return nil, sein.ErrForbidden("invalid or missing CSRF token")
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			method := strings.ToUpper(req.Method())
			cookieToken, _ := req.Cookie(cfg.CookieName)

			var tokenToSet string
			if cookieToken == "" {
				tokenToSet = generateToken()
				cookieToken = tokenToSet
			}

			// 1. Safe methods: bypass token verification, refresh cookie if needed
			if slices.Contains(safeMethods, method) {
				res, err := next(req)
				if err != nil {
					return nil, err
				}

				if tokenToSet != "" {
					return attachCSRFCookie(res, cfg, tokenToSet), nil
				}

				return res, nil
			}

			// 2. Mutating methods: extract client-submitted token
			clientToken := req.Header(cfg.HeaderName)
			if clientToken == "" {
				clientToken = req.Header("X-XSRF-Token")
			}

			if clientToken == "" {
				clientToken = req.FormValue(cfg.FormField)
			}

			if clientToken == "" {
				clientToken = string(req.Query(cfg.FormField))
			}

			// Constant-time token comparison
			if clientToken == "" || subtle.ConstantTimeCompare([]byte(clientToken), []byte(cookieToken)) != 1 {
				return cfg.ErrorHandler(req)
			}

			res, err := next(req)
			if err != nil {
				return nil, err
			}

			if tokenToSet != "" {
				return attachCSRFCookie(res, cfg, tokenToSet), nil
			}

			return res, nil
		}
	}
}

func attachCSRFCookie(res any, cfg Config, token string) any {
	// #nosec G124 -- CSRF cookie attributes are user configurable and SameSite is Lax by default
	cookie := &http.Cookie{
		Name:     cfg.CookieName,
		Value:    token,
		Path:     cfg.CookiePath,
		Domain:   cfg.CookieDomain,
		MaxAge:   int(cfg.Expiration.Seconds()),
		SameSite: cfg.CookieSameSite,
		Secure:   cfg.CookieSecure,
		HttpOnly: cfg.CookieHTTPOnly,
	}

	if holder, ok := res.(sein.ResponseHolder); ok {
		resp := sein.OK[any](holder.ResponseBody()).
			WithStatus(holder.StatusCode()).
			WithCookie(cookie)

		for k, vv := range holder.ResponseHeaders() {
			for _, v := range vv {
				resp = resp.WithHeader(k, v)
			}
		}

		return resp
	}

	return sein.OK[any](res).WithCookie(cookie)
}
