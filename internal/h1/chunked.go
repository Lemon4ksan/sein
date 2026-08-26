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
	"strconv"
)

var (
	ErrInvalidChunkSize   = errors.New("h1: invalid chunk size in chunked encoding")
	ErrChunkBoundaryError = errors.New("h1: missing CRLF at chunk boundary")
)

// ChunkedReader decodes an HTTP/1.1 chunked transfer-encoded byte stream.
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

		chunkSize, err := strconv.ParseInt(string(line), 16, 64)
		if err != nil || chunkSize < 0 {
			return 0, fmt.Errorf("%w: %v", ErrInvalidChunkSize, err)
		}

		if chunkSize == 0 {
			cr.done = true
			// Drain trailing CRLF or trailer headers
			_, _ = cr.r.ReadBytes('\n')
			return 0, io.EOF
		}

		cr.remaining = chunkSize
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
	_, _ = cw.w.WriteString(strconv.FormatInt(int64(len(p)), 16))
	_, _ = cw.w.WriteString("\r\n")
	_, _ = cw.w.Write(p)
	_, _ = cw.w.WriteString("\r\n")
	return len(p), cw.w.Flush()
}

// Close writes the terminal chunk "0\r\n\r\n" and flushes.
func (cw *ChunkedWriter) Close() error {
	_, _ = cw.w.WriteString("0\r\n\r\n")
	return cw.w.Flush()
}
