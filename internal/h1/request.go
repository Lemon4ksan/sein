// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
)

var (
	ErrMalformedRequestLine = errors.New("h1: malformed request line")
	ErrUnsupportedProtocol  = errors.New("h1: unsupported protocol version")
	ErrBodyTooLarge         = errors.New("h1: request body exceeds maximum allowed size")
	ErrHijackNotSupported   = errors.New("h1: hijacking not supported on this connection")
)

// Request holds parsed HTTP/1.1 request data without net/http wrapping.
type Request struct {
	Method     string
	URI        string
	Path       string
	Query      string
	Proto      string
	Host       string
	Headers    Headers
	Body       []byte
	RemoteAddr string
	TLS        *tls.ConnectionState
	HijackFn   func() (net.Conn, *bufio.ReadWriter, error)
}

// Reset clears the request structure for pooling.
func (r *Request) Reset() {
	r.Method = ""
	r.URI = ""
	r.Path = ""
	r.Query = ""
	r.Proto = ""
	r.Host = ""
	r.Headers.Reset()
	r.Body = r.Body[:0]
	r.RemoteAddr = ""
	r.TLS = nil
	r.HijackFn = nil
}

// Hijack takes over the raw network connection from the server.
func (r *Request) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if r.HijackFn == nil {
		return nil, nil, ErrHijackNotSupported
	}
	return r.HijackFn()
}

// ReadRequest parses an incoming HTTP/1.1 request from the buffered stream.
func (r *Request) ReadRequest(br *bufio.Reader, bw *bufio.Writer, maxBodySize int64) error {
	// 1. Read request line
	line, err := br.ReadBytes('\n')
	if err != nil {
		return err
	}

	line = bytes.TrimRight(line, "\r\n")
	if len(line) == 0 {
		return io.EOF
	}

	// Parse "METHOD URI PROTO"
	firstSpace := bytes.IndexByte(line, ' ')
	if firstSpace <= 0 {
		return ErrMalformedRequestLine
	}

	secondSpace := bytes.IndexByte(line[firstSpace+1:], ' ')
	if secondSpace <= 0 {
		return ErrMalformedRequestLine
	}
	secondSpace += firstSpace + 1

	r.Method = bytesconv.B2S(line[:firstSpace])
	r.URI = bytesconv.B2S(line[firstSpace+1 : secondSpace])
	r.Proto = bytesconv.B2S(line[secondSpace+1:])

	// Split URI into Path and Query
	if qIdx := strings.IndexByte(r.URI, '?'); qIdx != -1 {
		r.Path = r.URI[:qIdx]
		r.Query = r.URI[qIdx+1:]
	} else {
		r.Path = r.URI
		r.Query = ""
	}

	// 2. Read headers
	for {
		headerLine, err := br.ReadBytes('\n')
		if err != nil {
			return err
		}

		headerLine = bytes.TrimRight(headerLine, "\r\n")
		if len(headerLine) == 0 {
			// Empty line marks end of headers
			break
		}

		r.Headers.ParseHeaderLine(headerLine)
	}

	r.Host = r.Headers.Get(header.Host)

	// 3. Handle "Expect: 100-continue" (RFC 7231 §5.1.1)
	if strings.EqualFold(r.Headers.Get(header.Expect), "100-continue") && bw != nil {
		_, _ = bw.WriteString("HTTP/1.1 100 Continue\r\n\r\n")
		_ = bw.Flush()
	}

	// 4. Read body if present
	if strings.EqualFold(r.Headers.Get(header.TransferEncoding), header.ValueChunked) {
		chunkedBody, err := ReadAllChunked(br, maxBodySize)
		if err != nil {
			return err
		}
		r.Body = chunkedBody
	} else if clStr := r.Headers.Get(header.ContentLength); clStr != "" {
		contentLength, err := strconv.ParseInt(clStr, 10, 64)
		if err != nil || contentLength < 0 {
			return fmt.Errorf("h1: invalid content-length %q", clStr)
		}
		if contentLength > maxBodySize {
			return ErrBodyTooLarge
		}
		if contentLength > 0 {
			if cap(r.Body) < int(contentLength) {
				r.Body = make([]byte, contentLength)
			} else {
				r.Body = r.Body[:contentLength]
			}
			if _, err := io.ReadFull(br, r.Body); err != nil {
				return err
			}
		}
	}

	return nil
}

// ClientIP extracts the client IP address from remote address.
func (r *Request) ClientIP() string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
