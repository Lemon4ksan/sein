// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein_test

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/internal/compress"
	"github.com/lemon4ksan/sein/internal/fast/h1engine"
)

func BenchmarkH1_SIMDRequestParsing(b *testing.B) {
	rawHTTP := []byte(
		"GET /api/v1/users?limit=100&offset=0 HTTP/1.1\r\nHost: example.com\r\nUser-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64)\r\nAccept: application/json\r\nAccept-Encoding: gzip, deflate, br, zstd\r\nConnection: keep-alive\r\n\r\n",
	)

	rdr := bytes.NewReader(rawHTTP)
	br := bufio.NewReaderSize(rdr, 4096)
	req := &h1engine.Request{
		Headers: h1engine.NewHeadersWithCapacity(16),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req.Reset()
		rdr.Reset(rawHTTP)
		br.Reset(rdr)
		_ = req.ReadRequest(br, nil, 1024*1024)
	}
}

func BenchmarkH1_ResponseSerialization(b *testing.B) {
	res := sein.OK([]byte("Hello, World!"))

	var buf bytes.Buffer
	buf.Grow(512)
	bw := bufio.NewWriter(&buf)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		bw.Reset(&buf)

		_ = res.WriteResponse(httptest.NewRecorder())
	}
}

func BenchmarkServer_PlaintextRoute(b *testing.B) {
	app := sein.New()
	app.Get("/plaintext", func(ctx context.Context) (string, error) {
		return "Hello, World!", nil
	})

	req, _ := http.NewRequest(http.MethodGet, "/plaintext", nil)
	rw := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rw.Body.Reset()
		app.ServeHTTP(rw, req)
	}
}

func BenchmarkRouter_StaticMatch(b *testing.B) {
	router := sein.NewRouter()
	router.Add("GET", "/api/v1/users/profile", func(req *sein.Request) (any, error) {
		return "OK", nil
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = router.Match("GET", "/api/v1/users/profile", nil)
	}
}

func BenchmarkRouter_ParamMatch(b *testing.B) {
	router := sein.NewRouter()
	router.Add("GET", "/api/v1/users/:id/posts/:post_id", func(req *sein.Request) (any, error) {
		return "OK", nil
	})

	var params sein.Params

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		params.Reset()
		_, _ = router.Match("GET", "/api/v1/users/42/posts/100", &params)
	}
}

func BenchmarkZstd_Compress_Fastest(b *testing.B) {
	payload := []byte(
		`{"status":"ok","code":200,"message":"Multi-algorithm server compression benchmark payload with repeated JSON fields","items":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = compress.CompressZstd(payload, compress.ZstdSpeedFastest)
	}
}

func BenchmarkZstd_Compress_Default(b *testing.B) {
	payload := []byte(
		`{"status":"ok","code":200,"message":"Multi-algorithm server compression benchmark payload with repeated JSON fields","items":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = compress.CompressZstd(payload, compress.ZstdSpeedDefault)
	}
}

func BenchmarkBrotli_Compress_BestSpeed(b *testing.B) {
	payload := []byte(
		`{"status":"ok","code":200,"message":"Multi-algorithm server compression benchmark payload with repeated JSON fields","items":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = compress.CompressBrotli(payload, compress.BrotliBestSpeed)
	}
}

func BenchmarkBrotli_Compress_Default(b *testing.B) {
	payload := []byte(
		`{"status":"ok","code":200,"message":"Multi-algorithm server compression benchmark payload with repeated JSON fields","items":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = compress.CompressBrotli(payload, compress.BrotliDefaultCompression)
	}
}

func BenchmarkGzip_Compress_Default(b *testing.B) {
	payload := []byte(
		`{"status":"ok","code":200,"message":"Multi-algorithm server compression benchmark payload with repeated JSON fields","items":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = compress.CompressGzip(payload, 6)
	}
}
