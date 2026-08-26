// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"strings"

	"github.com/lemon4ksan/foundation/silicon/pool"

	"github.com/lemon4ksan/sein/internal/compress/brotli"
	"github.com/lemon4ksan/sein/internal/compress/zstd"
)

var (
	ErrUnsupportedEncoding = errors.New("compress: unsupported content encoding")
	ErrDecompressionFailed = errors.New("compress: decompression failed")
)

const (
	// Brotli compression levels
	BrotliBestSpeed          = brotli.BestSpeed          // Quality 0 (fastest)
	BrotliDefaultCompression = brotli.DefaultCompression // Quality 6 (sweet spot for HTTP)
	BrotliBestCompression    = brotli.BestCompression    // Quality 11 (max compression)

	// Zstd compression levels
	ZstdSpeedFastest           = zstd.SpeedFastest
	ZstdSpeedDefault           = zstd.SpeedDefault
	ZstdSpeedBetterCompression = zstd.SpeedBetterCompression
	ZstdSpeedBestCompression   = zstd.SpeedBestCompression
)

var (
	brotliWriterStorage = pool.NewPerPStorage(func() *brotli.Writer {
		return brotli.NewWriterLevel(io.Discard, BrotliDefaultCompression)
	})
	brotliFastWriterStorage = pool.NewPerPStorage(func() *brotli.Writer {
		return brotli.NewWriterLevel(io.Discard, BrotliBestSpeed)
	})
	brotliReaderStorage = pool.NewPerPStorage(func() *brotli.Reader {
		return brotli.NewReader(bytes.NewReader(nil))
	})

	gzipWriterStorage = pool.NewPerPStorage(func() *gzip.Writer {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		return w
	})
	gzipReaderStorage = pool.NewPerPStorage(func() *gzip.Reader {
		return new(gzip.Reader)
	})

	flateWriterStorage = pool.NewPerPStorage(func() *flate.Writer {
		w, _ := flate.NewWriter(io.Discard, flate.DefaultCompression)
		return w
	})

	zstdDefaultEncoderStorage = pool.NewPerPStorage(func() *zstd.Encoder {
		enc, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault), zstd.WithEncoderConcurrency(1))
		return enc
	})
	zstdFastEncoderStorage = pool.NewPerPStorage(func() *zstd.Encoder {
		enc, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest), zstd.WithEncoderConcurrency(1))
		return enc
	})
	zstdDecoderStorage = pool.NewPerPStorage(func() *zstd.Decoder {
		dec, _ := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
		return dec
	})
)

// CompressBrotli encodes src into compressed Brotli format.
func CompressBrotli(src []byte, level int) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	var (
		buf bytes.Buffer
		w   *brotli.Writer
	)

	switch level {
	case BrotliBestSpeed:
		w = brotliFastWriterStorage.Get()
		defer brotliFastWriterStorage.Put(w)
	case BrotliDefaultCompression:
		w = brotliWriterStorage.Get()
		defer brotliWriterStorage.Put(w)
	default:
		w = brotli.NewWriterLevel(&buf, level)
		defer w.Close()

		_, err := w.Write(src)
		if err != nil {
			return nil, err
		}

		if err := w.Close(); err != nil {
			return nil, err
		}

		return buf.Bytes(), nil
	}

	w.Reset(&buf)

	if _, err := w.Write(src); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DecompressBrotli decodes compressed Brotli src bytes.
func DecompressBrotli(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	r := brotliReaderStorage.Get()
	defer brotliReaderStorage.Put(r)

	if err := r.Reset(bytes.NewReader(src)); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// CompressGzip encodes src into compressed Gzip format.
func CompressGzip(src []byte, level int) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	var (
		buf bytes.Buffer
		w   *gzip.Writer
	)

	if level == gzip.DefaultCompression {
		w = gzipWriterStorage.Get()
		defer gzipWriterStorage.Put(w)

		w.Reset(&buf)
	} else {
		var err error

		w, err = gzip.NewWriterLevel(&buf, level)
		if err != nil {
			return nil, err
		}
	}

	if _, err := w.Write(src); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DecompressGzip decodes compressed Gzip src bytes.
func DecompressGzip(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	r := gzipReaderStorage.Get()
	defer gzipReaderStorage.Put(r)

	if err := r.Reset(bytes.NewReader(src)); err != nil {
		return nil, err
	}

	defer r.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// CompressZstd encodes src into compressed Zstandard format.
func CompressZstd(src []byte, level zstd.EncoderLevel) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	var enc *zstd.Encoder
	switch level {
	case ZstdSpeedFastest:
		enc = zstdFastEncoderStorage.Get()
		defer zstdFastEncoderStorage.Put(enc)
	case ZstdSpeedDefault:
		enc = zstdDefaultEncoderStorage.Get()
		defer zstdDefaultEncoderStorage.Put(enc)
	default:
		var err error

		enc, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(level), zstd.WithEncoderConcurrency(1))
		if err != nil {
			return nil, err
		}
		defer enc.Close()
	}

	return enc.EncodeAll(src, make([]byte, 0, len(src))), nil
}

// DecompressZstd decodes compressed Zstandard src bytes.
func DecompressZstd(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	dec := zstdDecoderStorage.Get()
	defer zstdDecoderStorage.Put(dec)

	return dec.DecodeAll(src, nil)
}

// CompressDeflate encodes src into raw DEFLATE format.
func CompressDeflate(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	w := flateWriterStorage.Get()
	defer flateWriterStorage.Put(w)

	w.Reset(&buf)
	if _, err := w.Write(src); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Decompress automatically decodes src based on Content-Encoding header value.
func Decompress(encoding string, src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	encoding = strings.TrimSpace(strings.ToLower(encoding))
	switch encoding {
	case "zstd":
		return DecompressZstd(src)
	case "br":
		return DecompressBrotli(src)
	case "gzip", "x-gzip":
		return DecompressGzip(src)
	case "deflate":
		zr := flate.NewReader(bytes.NewReader(src))
		defer zr.Close()

		var buf bytes.Buffer
		// #nosec G110 -- Capped buffer read protects against decompression bombs
		if _, err := io.Copy(&buf, io.LimitReader(zr, 64<<20)); err != nil {
			return nil, err
		}

		return buf.Bytes(), nil

	case "", "identity":
		return src, nil
	default:
		return nil, ErrUnsupportedEncoding
	}
}

// NewBrotliWriter returns a pooled Brotli writer wrapping w.
func NewBrotliWriter(w io.Writer, level int) *brotli.Writer {
	if level == BrotliBestSpeed {
		bw := brotliFastWriterStorage.Get()
		bw.Reset(w)
		return bw
	}

	bw := brotliWriterStorage.Get()
	bw.Reset(w)

	return bw
}

// ReleaseBrotliWriter returns a pooled Brotli writer.
func ReleaseBrotliWriter(bw *brotli.Writer, level int) {
	if bw == nil {
		return
	}

	_ = bw.Close()
	if level == BrotliBestSpeed {
		brotliFastWriterStorage.Put(bw)
	} else {
		brotliWriterStorage.Put(bw)
	}
}

// NewZstdWriter returns a pooled Zstandard writer wrapping w.
func NewZstdWriter(w io.Writer, level zstd.EncoderLevel) *zstd.Encoder {
	var enc *zstd.Encoder
	if level == ZstdSpeedFastest {
		enc = zstdFastEncoderStorage.Get()
	} else {
		enc = zstdDefaultEncoderStorage.Get()
	}

	enc.Reset(w)

	return enc
}

// ReleaseZstdWriter closes and returns a pooled Zstandard writer.
func ReleaseZstdWriter(zw *zstd.Encoder, level zstd.EncoderLevel) {
	if zw == nil {
		return
	}

	_ = zw.Close()
	if level == ZstdSpeedFastest {
		zstdFastEncoderStorage.Put(zw)
	} else {
		zstdDefaultEncoderStorage.Put(zw)
	}
}
