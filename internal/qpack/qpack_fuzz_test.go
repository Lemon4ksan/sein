// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qpack

import (
	"testing"
)

func FuzzQPACKDecoder(f *testing.F) {
	f.Add([]byte{0x00, 0x00, 0xd1}) // static index 17 (:status: 200)
	f.Add([]byte{0x00, 0x00, 0x51, 0x0b, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm'})
	f.Add([]byte{0x00, 0x00, 0x27, 0x03, 'c', 'u', 's', 0x03, 'v', 'a', 'l'})

	f.Fuzz(func(t *testing.T, data []byte) {
		dec := NewDecoder()
		_ = dec.DecodeFields(data, func(hf HeaderField) bool {
			return true
		})
	})
}

func FuzzVarint(f *testing.F) {
	f.Add([]byte{0x00}, byte(8))
	f.Add([]byte{0xff, 0x01}, byte(6))
	f.Add([]byte{0x1f, 0x9a, 0x0a}, byte(5))

	f.Fuzz(func(t *testing.T, data []byte, n byte) {
		if n == 0 || n > 8 {
			return
		}
		_, _, _ = readInt(n, data)
	})
}
