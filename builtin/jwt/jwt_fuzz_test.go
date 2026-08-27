// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jwt_test

import (
	"testing"

	"github.com/lemon4ksan/sein/builtin/jwt"
)

func FuzzJWTVerify(f *testing.F) {
	f.Add("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c", "secret")
	f.Add("invalid.token.here", "key")
	f.Add("eyJhbGciOiJub25lIn0.eyJzdWIiOiIxMjM0In0.", "")

	f.Fuzz(func(t *testing.T, token string, secret string) {
		_, _ = jwt.Parse(token, []byte(secret))
	})
}
