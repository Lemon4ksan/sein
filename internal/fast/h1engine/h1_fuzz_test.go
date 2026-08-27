// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1engine_test

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/sein/internal/fast/h1engine"
)

func FuzzH1Request(f *testing.F) {
	f.Add([]byte("GET /index.html HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	f.Add([]byte("POST /api/data HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\nhello"))
	f.Add([]byte("GET /search?q=test HTTP/1.1\r\nHost: example.com\r\nUser-Agent: Fuzzer\r\n\r\n"))
	f.Add([]byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nWiki\r\n5\r\npedia\r\n0\r\n\r\n"))
	f.Add([]byte("GET / HTTP/1.0\r\n\r\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		var req h1engine.Request
		br := bufio.NewReader(bytes.NewReader(data))
		_ = req.ReadRequest(br, nil, 1024*1024)
	})
}

func FuzzH1Chunked(f *testing.F) {
	f.Add([]byte("4\r\nWiki\r\n5\r\npedia\r\n0\r\n\r\n"))
	f.Add([]byte("0\r\n\r\n"))
	f.Add([]byte("1a\r\nabcdefghijklmnopqrstuvwxyz\r\n0\r\n\r\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		br := bufio.NewReader(bytes.NewReader(data))
		r := h1engine.NewChunkedReader(br)
		_, _ = io.ReadAll(r)
	})
}

func FuzzH1Header(f *testing.F) {
	f.Add("Host", "example.com")
	f.Add("Content-Type", "application/json; charset=utf-8")
	f.Add("X-Custom-Header", "value1, value2; q=0.8")
	f.Add("Authorization", "Bearer eyJhbGciOiJIUzI1NiJ9")

	f.Fuzz(func(t *testing.T, key, val string) {
		var headers h1engine.Headers
		headers.Set(key, val)
		_ = headers.Get(key)
		headers.Add(key, val)
	})
}
