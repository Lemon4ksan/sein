// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package skip provides conditional execution wrapper middleware.
// If the predicate evaluation returns true, the wrapped middleware is bypassed with zero overhead.
package skip

import (
	"github.com/lemon4ksan/sein"
)

// Predicate defines a condition function evaluated on each incoming request.
type Predicate func(req *sein.Request) bool

// New wraps a target middleware with a predicate condition.
// When the predicate returns true, the target middleware is skipped and execution proceeds to the next handler.
func New(mw sein.Middleware, predicate Predicate) sein.Middleware {
	if predicate == nil {
		return mw
	}

	return func(next sein.RawHandler) sein.RawHandler {
		wrapped := mw(next)

		return func(req *sein.Request) (any, error) {
			if predicate(req) {
				return next(req)
			}

			return wrapped(req)
		}
	}
}
