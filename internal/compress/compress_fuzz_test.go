// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/sein/internal/compress"
)

func FuzzGzipRoundtrip(f *testing.F) {
	f.Add([]byte("hello world compress payload"))
	f.Add([]byte(""))
	f.Add(make([]byte, 1024))

	f.Fuzz(func(t *testing.T, data []byte) {
		compressed, err := compress.CompressGzip(data, 6)
		if err != nil {
			return
		}
		decompressed, err := compress.DecompressGzip(compressed)
		if err != nil {
			t.Fatalf("failed to decompress valid gzip: %v", err)
		}
		if !bytes.Equal(data, decompressed) {
			t.Fatalf("decompressed mismatch")
		}
	})
}

func FuzzDecompressLimit(f *testing.F) {
	f.Add([]byte("arbitrary compressed or malformed payload"), int64(1024))
	f.Add([]byte(""), int64(0))

	f.Fuzz(func(t *testing.T, data []byte, limit int64) {
		if limit < 0 || limit > 1024*1024 {
			return
		}
		_, _ = compress.DecompressLimit("gzip", data, limit)
		_, _ = compress.DecompressLimit("zstd", data, limit)
		_, _ = compress.DecompressLimit("br", data, limit)
	})
}
