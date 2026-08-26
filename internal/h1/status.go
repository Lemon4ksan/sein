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
	nowBytes := []byte("Date: " + time.Now().UTC().Format(http.TimeFormat) + "\r\n")
	cachedDateHeader.Store(&nowBytes)
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
