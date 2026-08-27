// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cors

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// Config defines the configuration for the CORS middleware.
type Config struct {
	// AllowOrigins is a list of origins that are allowed to make requests.
	// Can contain "*", exact origin (e.g. "https://example.com"), or wildcard patterns (e.g. "https://*.example.com").
	// Default is ["*"].
	AllowOrigins []string

	// AllowMethods is a list of allowed HTTP methods.
	// Default is ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"].
	AllowMethods []string

	// AllowHeaders is a list of allowed request headers in preflight requests.
	// Default is ["*"] or common headers if not specified.
	AllowHeaders []string

	// ExposeHeaders is a list of response headers exposed to the client browser.
	ExposeHeaders []string

	// AllowCredentials indicates whether the request can include user credentials (cookies, authorization headers).
	// Note: If AllowCredentials is true, AllowOrigin cannot be "*".
	AllowCredentials bool

	// MaxAge indicates how many seconds the results of a preflight request can be cached (Access-Control-Max-Age).
	MaxAge int
}

// DefaultConfig is the default CORS configuration allowing all origins and methods.
var DefaultConfig = Config{
	AllowOrigins: []string{"*"},
	AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
	AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
	MaxAge:       86400,
}

// Default returns a CORS middleware with default configuration.
func Default() sein.Middleware {
	return New(DefaultConfig)
}

// New returns a new CORS middleware with the specified configuration.
func New(config ...Config) sein.Middleware {
	cfg := DefaultConfig
	if len(config) > 0 {
		cfg = config[0]
		if len(cfg.AllowOrigins) == 0 {
			cfg.AllowOrigins = DefaultConfig.AllowOrigins
		}

		if len(cfg.AllowMethods) == 0 {
			cfg.AllowMethods = DefaultConfig.AllowMethods
		}
	}

	allowMethodsHeader := strings.Join(cfg.AllowMethods, ", ")
	allowHeadersHeader := strings.Join(cfg.AllowHeaders, ", ")
	exposeHeadersHeader := strings.Join(cfg.ExposeHeaders, ", ")

	maxAgeHeader := ""
	if cfg.MaxAge > 0 {
		maxAgeHeader = strconv.Itoa(cfg.MaxAge)
	}

	allowAllOrigins := len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*"

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			origin := req.Header(header.Origin)
			if origin == "" {
				// Not a CORS request, pass through
				return next(req)
			}

			// Validate origin
			matchedOrigin := ""
			if allowAllOrigins {
				if cfg.AllowCredentials {
					matchedOrigin = origin
				} else {
					matchedOrigin = "*"
				}
			} else {
				for _, allowed := range cfg.AllowOrigins {
					if matchOrigin(allowed, origin) {
						matchedOrigin = origin
						break
					}
				}
			}

			if matchedOrigin == "" {
				// Origin not allowed
				return next(req)
			}

			// Handle preflight OPTIONS request
			if req.Method() == http.MethodOptions && req.Header(header.AccessControlRequestMethod) != "" {
				headers := make(http.Header)
				headers.Set(header.AccessControlAllowOrigin, matchedOrigin)

				if cfg.AllowCredentials {
					headers.Set(header.AccessControlAllowCredentials, "true")
				}

				if allowMethodsHeader != "" {
					headers.Set(header.AccessControlAllowMethods, allowMethodsHeader)
				}

				reqHeaders := req.Header(header.AccessControlRequestHeaders)
				if allowHeadersHeader != "" {
					headers.Set(header.AccessControlAllowHeaders, allowHeadersHeader)
				} else if reqHeaders != "" {
					headers.Set(header.AccessControlAllowHeaders, reqHeaders)
				}

				if maxAgeHeader != "" {
					headers.Set(header.AccessControlMaxAge, maxAgeHeader)
				}

				if exposeHeadersHeader != "" {
					headers.Set(header.AccessControlExposeHeaders, exposeHeadersHeader)
				}

				headers.Set(header.Vary, header.Origin)

				return sein.StatusWith[any](http.StatusNoContent, nil, headers), nil
			}

			// Execute downstream handler
			result, err := next(req)
			if err != nil {
				return nil, err
			}

			// Attach CORS headers to response
			return attachCORSHeaders(result, matchedOrigin, cfg.AllowCredentials, exposeHeadersHeader), nil
		}
	}
}

func attachCORSHeaders(result any, matchedOrigin string, allowCredentials bool, exposeHeaders string) any {
	if holder, ok := result.(sein.ResponseHolder); ok {
		headers := holder.ResponseHeaders()
		if headers == nil {
			headers = make(http.Header)
		}

		headers.Set(header.AccessControlAllowOrigin, matchedOrigin)

		if allowCredentials {
			headers.Set(header.AccessControlAllowCredentials, "true")
		}

		if exposeHeaders != "" {
			headers.Set(header.AccessControlExposeHeaders, exposeHeaders)
		}

		headers.Add(header.Vary, header.Origin)

		return sein.StatusWith(holder.StatusCode(), holder.ResponseBody(), headers)
	}

	headers := make(http.Header)
	headers.Set(header.AccessControlAllowOrigin, matchedOrigin)

	if allowCredentials {
		headers.Set(header.AccessControlAllowCredentials, "true")
	}

	if exposeHeaders != "" {
		headers.Set(header.AccessControlExposeHeaders, exposeHeaders)
	}

	headers.Add(header.Vary, header.Origin)

	return sein.StatusWith(http.StatusOK, result, headers)
}

func matchOrigin(pattern, origin string) bool {
	if pattern == "*" || pattern == origin {
		return true
	}

	if prefix, suffix, found := strings.Cut(pattern, "*"); found {
		return strings.HasPrefix(origin, prefix) && strings.HasSuffix(origin, suffix)
	}

	return false
}
