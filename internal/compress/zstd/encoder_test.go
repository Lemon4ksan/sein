// Copyright (c) 2019-2023 Klaus Post. All rights reserved.
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zstd_test

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"github.com/lemon4ksan/sein/internal/compress/zstd"
)

func generateTestData(size int, entropy float64) []byte {
	b := make([]byte, size)
	if size == 0 {
		return b
	}
	if entropy <= 0 {
		for i := range b {
			b[i] = byte('A' + (i % 26))
		}
		return b
	}
	if entropy >= 1.0 {
		_, _ = rand.Read(b)
		return b
	}
	// Mixed pattern
	chunk := make([]byte, 64)
	_, _ = rand.Read(chunk)
	for i := range b {
		b[i] = chunk[i%len(chunk)]
	}
	return b
}

func TestEncodeAllRoundtrip(t *testing.T) {
	t.Parallel()

	levels := []zstd.EncoderLevel{
		zstd.SpeedFastest,
		zstd.SpeedDefault,
		zstd.SpeedBetterCompression,
		zstd.SpeedBestCompression,
	}

	sizes := []int{0, 1, 2, 7, 15, 16, 64, 256, 1024, 4096, 65536, 131072, 524288}

	for _, lvl := range levels {
		for _, sz := range sizes {
			t.Run(fmt.Sprintf("level_%d_size_%d", lvl, sz), func(t *testing.T) {
				enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(lvl), zstd.WithEncoderConcurrency(1))
				if err != nil {
					t.Fatalf("failed to create encoder: %v", err)
				}
				defer enc.Close()

				dec, err := zstd.NewReader(nil)
				if err != nil {
					t.Fatalf("failed to create decoder: %v", err)
				}
				defer dec.Close()

				for _, entropy := range []float64{0.0, 0.5, 1.0} {
					raw := generateTestData(sz, entropy)

					compressed := enc.EncodeAll(raw, nil)
					if sz > 0 && len(compressed) == 0 {
						t.Fatalf("compressed output is empty for %d bytes", sz)
					}

					decompressed, err := dec.DecodeAll(compressed, nil)
					if err != nil {
						t.Fatalf("failed to decode (level %v, size %d, entropy %.1f): %v", lvl, sz, entropy, err)
					}

					if !bytes.Equal(raw, decompressed) {
						t.Fatalf("data mismatch (level %v, size %d, entropy %.1f)", lvl, sz, entropy)
					}
				}
			})
		}
	}
}

func TestStreamRoundtrip(t *testing.T) {
	t.Parallel()

	raw := generateTestData(256*1024, 0.4)

	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}

	chunkSize := 1024
	for i := 0; i < len(raw); i += chunkSize {
		end := min(i + chunkSize, len(raw))
		n, err := zw.Write(raw[i:end])
		if err != nil {
			t.Fatalf("write failed: %v", err)
		}
		if n != end-i {
			t.Fatalf("wrote %d bytes, want %d", n, end-i)
		}
	}

	if err := zw.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	zr, err := zstd.NewReader(&buf)
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}
	defer zr.Close()

	decompressed, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(raw, decompressed) {
		t.Fatalf("stream decompressed data does not match original (len %d vs %d)", len(decompressed), len(raw))
	}
}

func TestEncoderOptions(t *testing.T) {
	t.Parallel()

	raw := generateTestData(128*1024, 0.3)

	optCombinations := []struct {
		name string
		opts []zstd.EOption
	}{
		{"NoCRC", []zstd.EOption{zstd.WithEncoderCRC(false)}},
		{"SingleSegment", []zstd.EOption{zstd.WithSingleSegment(true)}},
		{"LowMem", []zstd.EOption{zstd.WithLowerEncoderMem(true)}},
		{"WindowSize", []zstd.EOption{zstd.WithWindowSize(64 * 1024)}},
		{"Concurrency2", []zstd.EOption{zstd.WithEncoderConcurrency(2)}},
		{"Concurrency4", []zstd.EOption{zstd.WithEncoderConcurrency(4)}},
		{"Padding1K", []zstd.EOption{zstd.WithEncoderPadding(1024)}},
		{"ZeroFrames", []zstd.EOption{zstd.WithZeroFrames(true)}},
	}

	dec, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	for _, tc := range optCombinations {
		t.Run(tc.name, func(t *testing.T) {
			enc, err := zstd.NewWriter(nil, tc.opts...)
			if err != nil {
				t.Fatalf("failed creating writer with %s: %v", tc.name, err)
			}
			defer enc.Close()

			comp := enc.EncodeAll(raw, nil)
			decomp, err := dec.DecodeAll(comp, nil)
			if err != nil {
				t.Fatalf("failed decoding %s: %v", tc.name, err)
			}
			if !bytes.Equal(raw, decomp) {
				t.Fatalf("mismatch for %s", tc.name)
			}
		})
	}
}

func TestEncoderReset(t *testing.T) {
	t.Parallel()

	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()

	dec, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()

	for i := 0; i < 5; i++ {
		raw := generateTestData((i+1)*16*1024, 0.2*float64(i))
		var buf bytes.Buffer
		enc.Reset(&buf)

		_, err := enc.Write(raw)
		if err != nil {
			t.Fatalf("iteration %d write failed: %v", i, err)
		}
		if err := enc.Close(); err != nil {
			t.Fatalf("iteration %d close failed: %v", i, err)
		}

		if err := dec.Reset(&buf); err != nil {
			t.Fatalf("iteration %d decoder reset failed: %v", i, err)
		}

		got, err := io.ReadAll(dec)
		if err != nil {
			t.Fatalf("iteration %d read failed: %v", i, err)
		}
		if !bytes.Equal(raw, got) {
			t.Fatalf("iteration %d mismatch", i)
		}
	}
}

func BenchmarkEncoder(b *testing.B) {
	payload := generateTestData(64*1024, 0.3)
	levels := []zstd.EncoderLevel{
		zstd.SpeedFastest,
		zstd.SpeedDefault,
		zstd.SpeedBetterCompression,
		zstd.SpeedBestCompression,
	}

	for _, lvl := range levels {
		b.Run(fmt.Sprintf("Level_%d", lvl), func(b *testing.B) {
			enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(lvl), zstd.WithEncoderConcurrency(1))
			if err != nil {
				b.Fatal(err)
			}
			defer enc.Close()

			dst := make([]byte, 0, len(payload))
			b.SetBytes(int64(len(payload)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = enc.EncodeAll(payload, dst[:0])
			}
		})
	}
}
