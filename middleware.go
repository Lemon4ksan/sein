// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
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
