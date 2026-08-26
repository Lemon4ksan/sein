// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"
	"io"
	"net/http"
	"strconv"
	"time"

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
func (res *Response) WriteTo(bw *bufio.Writer, keepAlive bool) error {
	status := res.StatusCode
	if status == 0 {
		status = http.StatusOK
	}

	statusText := http.StatusText(status)
	if statusText == "" {
		statusText = "Unknown"
	}

	// 1. Status Line: "HTTP/1.1 200 OK\r\n"
	_, _ = bw.WriteString("HTTP/1.1 ")
	_, _ = bw.WriteString(strconv.Itoa(status))
	_ = bw.WriteByte(' ')
	_, _ = bw.WriteString(statusText)
	_, _ = bw.WriteString("\r\n")

	// 2. Default Headers (Date, Connection, Content-Length/Chunked)
	if res.Headers.Get(header.Date) == "" {
		_, _ = bw.WriteString("Date: ")
		_, _ = bw.WriteString(time.Now().UTC().Format(http.TimeFormat))
		_, _ = bw.WriteString("\r\n")
	}

	if keepAlive {
		if res.Headers.Get(header.Connection) == "" {
			_, _ = bw.WriteString("Connection: keep-alive\r\n")
		}
	} else {
		_, _ = bw.WriteString("Connection: close\r\n")
	}

	if res.StreamWriter != nil {
		if res.Headers.Get(header.TransferEncoding) == "" {
			_, _ = bw.WriteString("Transfer-Encoding: chunked\r\n")
		}
	} else if status != http.StatusNoContent && status != http.StatusNotModified {
		if res.Headers.Get(header.ContentLength) == "" && res.Headers.Get(header.TransferEncoding) == "" {
			_, _ = bw.WriteString("Content-Length: ")
			_, _ = bw.WriteString(strconv.Itoa(len(res.Body)))
			_, _ = bw.WriteString("\r\n")
		}
	}

	// 3. User Headers
	for _, entry := range res.Headers.Entries() {
		_, _ = bw.WriteString(entry.Key)
		_, _ = bw.WriteString(": ")
		_, _ = bw.WriteString(entry.Value)
		_, _ = bw.WriteString("\r\n")
	}

	// 4. Cookies
	for _, c := range res.Cookies {
		if c != nil {
			_, _ = bw.WriteString("Set-Cookie: ")
			_, _ = bw.WriteString(c.String())
			_, _ = bw.WriteString("\r\n")
		}
	}

	// End of Headers
	_, _ = bw.WriteString("\r\n")

	// 5. Streaming Body or Static Body
	if res.StreamWriter != nil {
		_ = bw.Flush()
		cw := NewChunkedWriter(bw)
		err := res.StreamWriter(cw)
		closeErr := cw.Close()
		if err != nil {
			return err
		}
		return closeErr
	}

	if len(res.Body) > 0 && status != http.StatusNoContent && status != http.StatusNotModified {
		_, _ = bw.Write(res.Body)
	}

	return bw.Flush()
}
