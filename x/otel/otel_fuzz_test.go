// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package otel

import (
	"testing"
)

func FuzzParseTraceParent(f *testing.F) {
	f.Add("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	f.Add("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")
	f.Add("01-invalid-version-here")
	f.Add("")

	f.Fuzz(func(t *testing.T, val string) {
		_, _, _, _ = parseTraceParent(val)
	})
}
