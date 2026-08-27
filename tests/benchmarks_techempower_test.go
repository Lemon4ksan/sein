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

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/internal/fast/h1engine"
)

type JSONMessage struct {
	Message string `json:"message"`
}

// BenchmarkTechEmpower_Plaintext_SeinServeHTTP measures end-to-end HTTP/1.1 Plaintext throughput via ServeHTTP.
func BenchmarkTechEmpower_Plaintext_SeinServeHTTP(b *testing.B) {
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

// BenchmarkTechEmpower_Plaintext_SeinDispatchH1 measures pure Native H1 Engine Plaintext throughput (0 net/http).
func BenchmarkTechEmpower_Plaintext_SeinDispatchH1(b *testing.B) {
	app := sein.New()
	app.Get("/plaintext", func(ctx context.Context) (string, error) {
		return "Hello, World!", nil
	})

	h1Req := &h1engine.Request{
		Method: "GET",
		Path:   "/plaintext",
	}
	h1Res := &h1engine.Response{
		Body: make([]byte, 0, 512),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h1Res.Reset()
		_ = app.DispatchH1(h1Req, h1Res)
	}
}

// BenchmarkTechEmpower_Plaintext_StdHTTP measures Standard Library http.ServeMux Plaintext throughput.
func BenchmarkTechEmpower_Plaintext_StdHTTP(b *testing.B) {
	mux := http.NewServeMux()
	mux.HandleFunc("/plaintext", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Hello, World!"))
	})

	req, _ := http.NewRequest(http.MethodGet, "/plaintext", nil)
	rw := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rw.Body.Reset()
		mux.ServeHTTP(rw, req)
	}
}

// BenchmarkTechEmpower_JSON_SeinServeHTTP measures end-to-end JSON serialization throughput via ServeHTTP.
func BenchmarkTechEmpower_JSON_SeinServeHTTP(b *testing.B) {
	app := sein.New()
	app.Get("/json", func(ctx context.Context) (JSONMessage, error) {
		return JSONMessage{Message: "Hello, World!"}, nil
	})

	req, _ := http.NewRequest(http.MethodGet, "/json", nil)
	rw := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rw.Body.Reset()
		app.ServeHTTP(rw, req)
	}
}

// BenchmarkTechEmpower_JSON_SeinDispatchH1 measures pure Native H1 Engine JSON serialization throughput.
func BenchmarkTechEmpower_JSON_SeinDispatchH1(b *testing.B) {
	app := sein.New()
	app.Get("/json", func(ctx context.Context) (JSONMessage, error) {
		return JSONMessage{Message: "Hello, World!"}, nil
	})

	h1Req := &h1engine.Request{
		Method: "GET",
		Path:   "/json",
	}
	h1Res := &h1engine.Response{
		Body: make([]byte, 0, 512),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h1Res.Reset()
		_ = app.DispatchH1(h1Req, h1Res)
	}
}

// BenchmarkTechEmpower_JSON_StdHTTP measures Standard Library JSON serialization throughput.
func BenchmarkTechEmpower_JSON_StdHTTP(b *testing.B) {
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(JSONMessage{Message: "Hello, World!"})
	})

	req, _ := http.NewRequest(http.MethodGet, "/json", nil)
	rw := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rw.Body.Reset()
		mux.ServeHTTP(rw, req)
	}
}

// BenchmarkTechEmpower_DynamicRoute_Sein measures dynamic parameterized route throughput (/user/:id).
func BenchmarkTechEmpower_DynamicRoute_Sein(b *testing.B) {
	app := sein.New()
	app.GetReq("/user/:id", func(req *sein.Request) (string, error) {
		return "User: " + req.Param("id").String(), nil
	})

	req, _ := http.NewRequest(http.MethodGet, "/user/1049285", nil)
	rw := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rw.Body.Reset()
		app.ServeHTTP(rw, req)
	}
}

// BenchmarkTechEmpower_FastH1Engine_PipelinedThroughput measures pure SIMD/Per-P Fast H1 engine throughput.
func BenchmarkTechEmpower_FastH1Engine_PipelinedThroughput(b *testing.B) {
	rawHTTP := []byte("GET /plaintext HTTP/1.1\r\nHost: localhost\r\n\r\n")

	ch := &h1engine.ConnHandler{
		Handler: func(req *h1engine.Request, res *h1engine.Response) error {
			res.StatusCode = http.StatusOK
			res.Headers.Set(header.ContentType, header.MIMETextPlainCharsetUTF8)
			res.Body = append(res.Body[:0], "Hello, World!"...)
			return nil
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		rdr := bytes.NewReader(rawHTTP)
		br := bufio.NewReaderSize(rdr, 4096)
		req := &h1engine.Request{
			Headers: h1engine.NewHeadersWithCapacity(8),
		}
		res := &h1engine.Response{
			Body: make([]byte, 0, 512),
		}

		for pb.Next() {
			req.Reset()
			res.Reset()
			rdr.Reset(rawHTTP)
			br.Reset(rdr)

			_ = req.ReadRequest(br, nil, 1024*1024)
			_ = ch.Handler(req, res)
		}
	})
}

// BenchmarkTechEmpower_Parallel_SeinDispatchH1 measures multi-core concurrent throughput of Native H1 Dispatcher.
func BenchmarkTechEmpower_Parallel_SeinDispatchH1(b *testing.B) {
	app := sein.New()
	app.Get("/plaintext", func(ctx context.Context) (string, error) {
		return "Hello, World!", nil
	})

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		h1Req := &h1engine.Request{
			Method: "GET",
			Path:   "/plaintext",
		}
		h1Res := &h1engine.Response{
			Body: make([]byte, 0, 512),
		}

		for pb.Next() {
			h1Res.Reset()
			_ = app.DispatchH1(h1Req, h1Res)
		}
	})
}

// BenchmarkTechEmpower_Parallel_Throughput measures concurrent multi-core throughput of Sein ServeHTTP.
func BenchmarkTechEmpower_Parallel_Throughput(b *testing.B) {
	app := sein.New()
	app.Get("/plaintext", func(ctx context.Context) (string, error) {
		return "Hello, World!", nil
	})

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		req, _ := http.NewRequest(http.MethodGet, "/plaintext", nil)
		rw := httptest.NewRecorder()

		for pb.Next() {
			rw.Body.Reset()
			app.ServeHTTP(rw, req)
		}
	})
}
