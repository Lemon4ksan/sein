// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sentry

import (
	"testing"
)

func FuzzSentryDSN(f *testing.F) {
	f.Add("https://public_key@sentry.io/12345")
	f.Add("http://key@localhost:9000/1")
	f.Add("invalid-dsn-string")
	f.Add("")

	f.Fuzz(func(t *testing.T, dsn string) {
		tr, err := newHTTPTransport(dsn)
		if err == nil && tr != nil {
			tr.cancel()
		}
	})
}
