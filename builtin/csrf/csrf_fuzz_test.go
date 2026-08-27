// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package csrf_test

import (
	"crypto/subtle"
	"testing"
)

func FuzzCSRFTokenComparison(f *testing.F) {
	f.Add("d8e8fca2dc0f896fd7cb4cb0031ba249", "d8e8fca2dc0f896fd7cb4cb0031ba249")
	f.Add("d8e8fca2dc0f896fd7cb4cb0031ba249", "badtoken")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, a, b string) {
		if len(a) != len(b) {
			return
		}
		_ = subtle.ConstantTimeCompare([]byte(a), []byte(b))
	})
}
