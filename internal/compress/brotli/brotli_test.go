// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package brotli_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein/internal/compress/brotli"
)

func TestBrotliRoundtrip(t *testing.T) {
	data := []byte(strings.Repeat("Brotli zero-allocation high throughput compression and decompression engine! ", 100))

	for _, level := range []int{brotli.BestSpeed, brotli.DefaultCompression, brotli.BestCompression} {
		t.Run("level", func(t *testing.T) {
			var buf bytes.Buffer
			w := brotli.NewWriterLevel(&buf, level)

			_, err := w.Write(data)
			require.NoError(t, err)
			require.NoError(t, w.Close())

			compressed := buf.Bytes()
			assert.True(t, len(compressed) < len(data))

			r := brotli.NewReader(bytes.NewReader(compressed))
			decompressed, err := io.ReadAll(r)
			require.NoError(t, err)
			assert.Equal(t, string(data), string(decompressed))
		})
	}
}

func TestBrotliWriterReset(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	w := brotli.NewWriter(&buf1)

	msg1 := []byte("first payload")
	_, err := w.Write(msg1)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	w.Reset(&buf2)
	msg2 := []byte("second payload with reset")
	_, err = w.Write(msg2)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	r1 := brotli.NewReader(bytes.NewReader(buf1.Bytes()))
	d1, err := io.ReadAll(r1)
	require.NoError(t, err)
	assert.Equal(t, string(msg1), string(d1))

	r2 := brotli.NewReader(bytes.NewReader(buf2.Bytes()))
	d2, err := io.ReadAll(r2)
	require.NoError(t, err)
	assert.Equal(t, string(msg2), string(d2))
}
