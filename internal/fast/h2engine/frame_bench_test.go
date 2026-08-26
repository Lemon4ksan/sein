// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2engine_test

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/lemon4ksan/sein/internal/fast/h2engine"
)

func BenchmarkFrameHeader_ReadFrame(b *testing.B) {
	raw := []byte{0x00, 0x00, 0x05, 0x01, 0x05, 0x00, 0x00, 0x00, 0x03, 'h', 'e', 'l', 'l', 'o'}
	rdr := bytes.NewReader(raw)
	br := bufio.NewReader(rdr)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rdr.Reset(raw)
		br.Reset(rdr)

		fr, err := h2engine.ReadFrameFrom(br)
		if err == nil {
			h2engine.ReleaseFrameHeader(fr)
		}
	}
}

func BenchmarkFrameHeader_Pack9Bytes(b *testing.B) {
	var dst [9]byte

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h2engine.PackFrameHeader(dst[:], 16384, h2engine.FrameHeaders, h2engine.FlagEndHeaders|h2engine.FlagEndStream, 13)
	}
}
