// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etag_test

import (
	"fmt"
	"hash/crc32"
	"strings"
	"testing"
)

func FuzzETagMatch(f *testing.F) {
	f.Add([]byte("sample body content"), "\"19-a1b2c3d4\"")
	f.Add([]byte("hello"), "W/\"5-12345678\"")
	f.Add([]byte(""), "*")

	f.Fuzz(func(t *testing.T, body []byte, clientETag string) {
		if len(body) == 0 {
			return
		}
		checksum := crc32.ChecksumIEEE(body)
		tagVal := fmt.Sprintf("\"%d-%08x\"", len(body), checksum)

		clientETag = strings.TrimSpace(clientETag)
		_ = (clientETag == "*" || clientETag == tagVal || strings.TrimPrefix(clientETag, "W/") == strings.TrimPrefix(tagVal, "W/"))
	})
}
