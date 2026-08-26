// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidChunkSize   = errors.New("h1: invalid chunk size in chunked encoding")
	ErrChunkBoundaryError = errors.New("h1: missing CRLF at chunk boundary")
	errEmptyHexNum        = errors.New("h1: empty hex number")

	_ = parseHexUintFallback
	_ = formatHexUintFallback
)

func parseHexUintFallback(src []byte) (int, int, error) {
	if len(src) == 0 {
		return 0, 0, errEmptyHexNum
	}

	var (
		val int
		i   int
	)
	for i = 0; i < len(src); i++ {
		c := src[i]

		var d int
		if c >= '0' && c <= '9' {
			d = int(c - '0')
		} else if c >= 'a' && c <= 'f' {
			d = int(c - 'a' + 10)
		} else if c >= 'A' && c <= 'F' {
			d = int(c - 'A' + 10)
		} else {
			if i == 0 {
				return 0, 0, errEmptyHexNum
			}

			break
		}

		val = (val << 4) | d
	}

	return val, i, nil
}

func formatHexUintFallback(buf *[16]byte, val int) int {
	if val == 0 {
		buf[0] = '0'
		return 1
	}

	idx := 15
	for val > 0 {
		nib := byte(val & 0x0F)
		if nib < 10 {
			buf[idx] = '0' + nib
		} else {
			buf[idx] = 'a' + (nib - 10)
		}

		idx--
		val >>= 4
	}

	count := 15 - idx
	copy(buf[:count], buf[idx+1:16])

	return count
}

// ChunkedReader decodes an HTTP/1.1 chunked transfer-encoded byte stream using SIMD hex parsing.
type ChunkedReader struct {
	r         *bufio.Reader
	remaining int64
	done      bool
}

// NewChunkedReader creates a new ChunkedReader wrapping the provided bufio.Reader.
func NewChunkedReader(r *bufio.Reader) *ChunkedReader {
	return &ChunkedReader{r: r}
}

// Read reads decoded data from the chunked stream.
func (cr *ChunkedReader) Read(p []byte) (n int, err error) {
	if cr.done {
		return 0, io.EOF
	}

	if cr.remaining == 0 {
		// Read next chunk header
		line, err := cr.r.ReadBytes('\n')
		if err != nil {
			return 0, err
		}

		line = bytes.TrimRight(line, "\r\n")
		// Strip chunk extensions if present (e.g., "1a;ext=val")
		if idx := bytes.IndexByte(line, ';'); idx != -1 {
			line = line[:idx]
		}

		line = bytes.TrimSpace(line)

		if len(line) == 0 {
			return 0, ErrInvalidChunkSize
		}

		chunkSize, _, err := vectorParseHexUint(line)
		if err != nil || chunkSize < 0 {
			return 0, fmt.Errorf("%w: %w", ErrInvalidChunkSize, err)
		}

		if chunkSize == 0 {
			cr.done = true
			// Drain trailing CRLF or trailer headers
			_, _ = cr.r.ReadBytes('\n')

			return 0, io.EOF
		}

		cr.remaining = int64(chunkSize)
	}

	toRead := min(int64(len(p)), cr.remaining)
	n, err = cr.r.Read(p[:toRead])
	cr.remaining -= int64(n)

	if cr.remaining == 0 && err == nil {
		// Expect CRLF after chunk data
		b1, err1 := cr.r.ReadByte()

		b2, err2 := cr.r.ReadByte()
		if err1 != nil || err2 != nil || b1 != '\r' || b2 != '\n' {
			return n, ErrChunkBoundaryError
		}
	}

	return n, err
}

// ReadAllChunked drains all chunked content into a preallocated byte slice.
func ReadAllChunked(r *bufio.Reader, maxBodySize int64) ([]byte, error) {
	cr := NewChunkedReader(r)

	var buf bytes.Buffer

	lr := io.LimitReader(cr, maxBodySize+1)

	_, err := buf.ReadFrom(lr)
	if err != nil {
		return nil, err
	}

	if int64(buf.Len()) > maxBodySize {
		return nil, errors.New("h1: request body exceeds maximum allowed size")
	}

	return buf.Bytes(), nil
}

// ChunkedWriter writes data using HTTP/1.1 chunked transfer encoding.
type ChunkedWriter struct {
	w *bufio.Writer
}

// NewChunkedWriter creates a new ChunkedWriter wrapping w.
func NewChunkedWriter(w *bufio.Writer) *ChunkedWriter {
	return &ChunkedWriter{w: w}
}

// Write frames p as a chunk: "<hex-length>\r\n<data>\r\n" and flushes.
func (cw *ChunkedWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	var hexBuf [16]byte

	n := vectorFormatHexUint(&hexBuf, len(p))

	_, _ = cw.w.Write(hexBuf[:n])
	_, _ = cw.w.Write(hdrCRLF)
	_, _ = cw.w.Write(p)
	_, _ = cw.w.Write(hdrCRLF)

	return len(p), cw.w.Flush()
}

// Close writes the terminal chunk "0\r\n\r\n" and flushes.
func (cw *ChunkedWriter) Close() error {
	_, _ = cw.w.WriteString("0\r\n\r\n")
	return cw.w.Flush()
}
