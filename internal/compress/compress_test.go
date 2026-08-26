// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lemon4ksan/sein/internal/compress"
)

func TestBrotliCompressionRoundtrip(t *testing.T) {
	data := []byte(strings.Repeat("Brotli server compression for JSON and API responses! ", 100))

	// 1. Fast speed (level 0)
	fastCompressed, err := compress.CompressBrotli(data, compress.BrotliBestSpeed)
	if err != nil {
		t.Fatalf("CompressBrotli BestSpeed failed: %v", err)
	}
	if len(fastCompressed) >= len(data) {
		t.Errorf("expected compression, original: %d, compressed: %d", len(data), len(fastCompressed))
	}

	fastDecompressed, err := compress.DecompressBrotli(fastCompressed)
	if err != nil {
		t.Fatalf("DecompressBrotli failed: %v", err)
	}
	if !bytes.Equal(fastDecompressed, data) {
		t.Errorf("decompressed data mismatch")
	}

	// 2. Default compression (level 6)
	defaultCompressed, err := compress.CompressBrotli(data, compress.BrotliDefaultCompression)
	if err != nil {
		t.Fatalf("CompressBrotli DefaultCompression failed: %v", err)
	}

	defaultDecompressed, err := compress.DecompressBrotli(defaultCompressed)
	if err != nil {
		t.Fatalf("DecompressBrotli failed: %v", err)
	}
	if !bytes.Equal(defaultDecompressed, data) {
		t.Errorf("decompressed data mismatch")
	}
}

func TestGzipCompressionRoundtrip(t *testing.T) {
	data := []byte(strings.Repeat("Gzip compatibility for legacy clients! ", 100))

	compressed, err := compress.CompressGzip(data, 6)
	if err != nil {
		t.Fatalf("CompressGzip failed: %v", err)
	}

	decompressed, err := compress.DecompressGzip(compressed)
	if err != nil {
		t.Fatalf("DecompressGzip failed: %v", err)
	}

	if !bytes.Equal(decompressed, data) {
		t.Errorf("decompressed data mismatch")
	}
}

func TestZstdCompressionRoundtrip(t *testing.T) {
	data := []byte(strings.Repeat("Zstandard blazing fast server compression with silicon acceleration! ", 100))

	// 1. Fast speed (level 1)
	fastCompressed, err := compress.CompressZstd(data, compress.ZstdSpeedFastest)
	if err != nil {
		t.Fatalf("CompressZstd Fastest failed: %v", err)
	}
	if len(fastCompressed) >= len(data) {
		t.Errorf("expected compression, original: %d, compressed: %d", len(data), len(fastCompressed))
	}

	fastDecompressed, err := compress.DecompressZstd(fastCompressed)
	if err != nil {
		t.Fatalf("DecompressZstd failed: %v", err)
	}
	if !bytes.Equal(fastDecompressed, data) {
		t.Errorf("decompressed data mismatch")
	}

	// 2. Default compression (level 2)
	defaultCompressed, err := compress.CompressZstd(data, compress.ZstdSpeedDefault)
	if err != nil {
		t.Fatalf("CompressZstd Default failed: %v", err)
	}

	defaultDecompressed, err := compress.DecompressZstd(defaultCompressed)
	if err != nil {
		t.Fatalf("DecompressZstd failed: %v", err)
	}
	if !bytes.Equal(defaultDecompressed, data) {
		t.Errorf("decompressed data mismatch")
	}
}

func TestAutoDecompress(t *testing.T) {
	data := []byte("Universal decompression test payload 2026")

	// 1. Test "zstd"
	zstdComp, _ := compress.CompressZstd(data, compress.ZstdSpeedDefault)
	zstdDec, err := compress.Decompress("zstd", zstdComp)
	if err != nil || !bytes.Equal(zstdDec, data) {
		t.Fatalf("auto decompress 'zstd' failed: %v", err)
	}

	// 2. Test "br"
	brComp, _ := compress.CompressBrotli(data, compress.BrotliDefaultCompression)
	brDec, err := compress.Decompress("br", brComp)
	if err != nil || !bytes.Equal(brDec, data) {
		t.Fatalf("auto decompress 'br' failed: %v", err)
	}

	// 3. Test "gzip"
	gzComp, _ := compress.CompressGzip(data, 6)
	gzDec, err := compress.Decompress("gzip", gzComp)
	if err != nil || !bytes.Equal(gzDec, data) {
		t.Fatalf("auto decompress 'gzip' failed: %v", err)
	}

	// 4. Test "identity"
	identDec, err := compress.Decompress("identity", data)
	if err != nil || !bytes.Equal(identDec, data) {
		t.Fatalf("auto decompress 'identity' failed: %v", err)
	}
}

