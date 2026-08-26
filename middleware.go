// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
)

// RawHandler is the internal uniform handler signature returning any payload or error.
type RawHandler func(req *Request) (any, error)

// Middleware wraps a RawHandler in an onion chain.
type Middleware func(next RawHandler) RawHandler

// Recovery returns a middleware that catches panics and turns them into 500 Internal Server Errors.
func Recovery() Middleware {
	return func(next RawHandler) RawHandler {
		return func(req *Request) (res any, err error) {
			defer func() {
				if r := recover(); r != nil {
					stack := string(debug.Stack())
					slog.Error("Panic recovered in sein handler",
						"panic", r,
						"path", req.Path(),
						"stack", stack,
					)
					err = ErrInternal(fmt.Sprintf("internal server panic: %v", r))
				}
			}()

			return next(req)
		}
	}
}

// BearerAuth returns a middleware that extracts the Bearer token, validates it using validator,
// and injects the returned session of type T into the request's L1-cache inline storage (0 B/op).
// If the token is missing or invalid, it immediately halts the pipeline with a 401 Unauthorized error.
func BearerAuth[T any](validator func(ctx context.Context, token string) (T, error)) Middleware {
	return func(next RawHandler) RawHandler {
		return func(req *Request) (any, error) {
			token, ok := req.BearerToken()
			if !ok || token == "" {
				return nil, ErrMissingBearerToken
			}

			session, err := validator(req.Context(), token)
			if err != nil {
				var domainErr DomainError
				if errors.As(err, &domainErr) {
					return nil, domainErr
				}

				return nil, ErrInvalidBearerToken.WithCause(err)
			}

			Set(req, session)

			return next(req)
		}
	}
}
