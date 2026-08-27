// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1engine

import (
	"bufio"
	"io"
	"net/http"
	"strconv"

	"github.com/lemon4ksan/foundation/net/http/header"
)

// Response carries HTTP/1.1 response state to be serialized directly over the wire.
type Response struct {
	StatusCode   int
	Headers      Headers
	Cookies      []*http.Cookie
	Body         []byte
	StreamWriter func(w io.Writer) error
}

// Reset clears the response for recycling.
func (res *Response) Reset() {
	res.StatusCode = http.StatusOK
	res.Headers.Reset()
	res.Cookies = res.Cookies[:0]
	res.Body = res.Body[:0]
	res.StreamWriter = nil
}

// WriteTo writes the full HTTP/1.1 response (status line, headers, cookies, body or stream) to the writer.
// If flush is false, bytes remain buffered in bw to coalesce pipelined responses into a single write syscall.
func (res *Response) WriteTo(bw *bufio.Writer, keepAlive bool, flush bool) error {
	status := res.StatusCode
	if status == 0 {
		status = http.StatusOK
	}

	// 1. Fast Status Line (from pre-compiled static table)
	if status >= 100 && status < len(statusLines) && statusLines[status] != nil {
		_, _ = bw.Write(statusLines[status])
	} else {
		statusText := http.StatusText(status)
		if statusText == "" {
			statusText = "Unknown"
		}

		var stBuf [16]byte
		_, _ = bw.WriteString("HTTP/1.1 ")
		_, _ = bw.Write(strconv.AppendInt(stBuf[:0], int64(status), 10))
		_ = bw.WriteByte(' ')
		_, _ = bw.WriteString(statusText)
		_, _ = bw.Write(hdrCRLF)
	}

	// 2. Atomic Cached Date Header
	if res.Headers.Get(header.Date) == "" {
		dateBytes := cachedDateHeader.Load()
		if dateBytes != nil {
			_, _ = bw.Write(*dateBytes)
		}
	}

	// 3. Connection Header
	if keepAlive {
		if res.Headers.Get(header.Connection) == "" {
			_, _ = bw.Write(hdrConnectionKeepAlive)
		}
	} else {
		_, _ = bw.Write(hdrConnectionClose)
	}

	// 4. Transfer-Encoding or Content-Length
	if res.StreamWriter != nil {
		if res.Headers.Get(header.TransferEncoding) == "" {
			_, _ = bw.Write(hdrTransferChunked)
		}
	} else if status != http.StatusNoContent && status != http.StatusNotModified {
		if res.Headers.Get(header.ContentLength) == "" && res.Headers.Get(header.TransferEncoding) == "" {
			var clBuf [24]byte
			_, _ = bw.Write(hdrContentLengthPrefix)
			_, _ = bw.Write(strconv.AppendInt(clBuf[:0], int64(len(res.Body)), 10))
			_, _ = bw.Write(hdrCRLF)
		}
	}

	// 5. User Headers
	for _, entry := range res.Headers.Entries() {
		_, _ = bw.WriteString(entry.Key)
		_, _ = bw.Write(hdrColonSpace)
		_, _ = bw.WriteString(entry.Value)
		_, _ = bw.Write(hdrCRLF)
	}

	// 6. Cookies
	for _, c := range res.Cookies {
		if c != nil {
			_, _ = bw.Write(hdrSetCookiePrefix)
			_, _ = bw.WriteString(c.String())
			_, _ = bw.Write(hdrCRLF)
		}
	}

	// End of Headers
	_, _ = bw.Write(hdrCRLF)

	// 7. Streaming Body or Static Body
	if res.StreamWriter != nil {
		_ = bw.Flush()
		cw := NewChunkedWriter(bw)
		err := res.StreamWriter(cw)
		closeErr := cw.Close()

		if err != nil {
			return err
		}

		if closeErr != nil {
			return closeErr
		}

		if flush {
			return bw.Flush()
		}

		return nil
	}

	if len(res.Body) > 0 && status != http.StatusNoContent && status != http.StatusNotModified {
		_, _ = bw.Write(res.Body)
	}

	if flush {
		return bw.Flush()
	}

	return nil
}
