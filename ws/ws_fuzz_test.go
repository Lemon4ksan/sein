// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws_test

import (
	"encoding/binary"
	"testing"

	"github.com/lemon4ksan/sein/ws"
)

func FuzzWSMask(f *testing.F) {
	f.Add([]byte("hello world websocket"), uint32(0x12345678))
	f.Add([]byte(""), uint32(0))
	f.Add(make([]byte, 128), uint32(0xffffffff))

	f.Fuzz(func(t *testing.T, payload []byte, maskUint uint32) {
		var mask [4]byte
		binary.LittleEndian.PutUint32(mask[:], maskUint)

		buf1 := make([]byte, len(payload))
		copy(buf1, payload)
		ws.VectorApplyFastMask(buf1, mask)

		buf2 := make([]byte, len(payload))
		copy(buf2, payload)
		ws.ApplyMask(buf2, mask)
	})
}

func FuzzWSAcceptKey(f *testing.F) {
	f.Add("dGhlIHNhbXBsZSBub25jZQ==")
	f.Add("AQIDBAUGBwgJCgsMDQ4PEA==")
	f.Add("")

	f.Fuzz(func(t *testing.T, key string) {
		_ = ws.ComputeAcceptKey(key)
	})
}
