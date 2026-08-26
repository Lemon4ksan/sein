// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/timekit"
)

var (
	statusLines [600][]byte

	cachedDateHeader atomic.Pointer[[]byte]
	dateTickerOnce   sync.Once

	hdrConnectionKeepAlive = []byte("Connection: keep-alive\r\n")
	hdrConnectionClose     = []byte("Connection: close\r\n")
	hdrTransferChunked     = []byte("Transfer-Encoding: chunked\r\n")
	hdrContentLengthPrefix = []byte("Content-Length: ")
	hdrCRLF                = []byte("\r\n")
	hdrColonSpace          = []byte(": ")
	hdrSetCookiePrefix     = []byte("Set-Cookie: ")
)

func init() {
	// Pre-compile all HTTP status line byte slices (100 to 599)
	for code := 100; code < 600; code++ {
		text := http.StatusText(code)
		if text != "" {
			statusLines[code] = []byte(fmt.Sprintf("HTTP/1.1 %d %s\r\n", code, text))
		}
	}

	initDateHeader()
}

func updateDateHeader() {
	buf := make([]byte, 0, 6+timekit.HTTPDateLength+2)
	buf = append(buf, "Date: "...)
	buf = timekit.AppendHTTPDate(buf, timekit.CoarseNow())
	buf = append(buf, "\r\n"...)
	cachedDateHeader.Store(&buf)
}

func initDateHeader() {
	dateTickerOnce.Do(func() {
		updateDateHeader()
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			for range ticker.C {
				updateDateHeader()
			}
		}()
	})
}
