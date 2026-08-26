// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compress_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"github.com/lemon4ksan/sein"
	intCompress "github.com/lemon4ksan/sein/internal/compress"
	"github.com/lemon4ksan/sein/x/compress"
)

func TestBrotliResponseCompression(t *testing.T) {
	app := sein.New()
	app.Use(compress.New(compress.WithMinLength(100)))

	largeText := strings.Repeat("Brotli is fast, efficient, and perfect for JSON APIs! ", 30)

	type UserData struct {
		Payload string `json:"payload"`
	}

	app.Get("/data", func(ctx context.Context) (UserData, error) {
		return UserData{Payload: largeText}, nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()

	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Request with Accept-Encoding: br
	req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/data", nil)
	req.Header.Set("Accept-Encoding", "br, gzip")
	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "br", resp.Header.Get("Content-Encoding"))

	compressedBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()

	// Decompress and verify
	decompressed, err := intCompress.DecompressBrotli(compressedBody)
	require.NoError(t, err)

	var result UserData
	require.NoError(t, json.Unmarshal(decompressed, &result))
	assert.Equal(t, largeText, result.Payload)

	// 2. Request with Accept-Encoding: gzip
	reqGzip, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/data", nil)
	reqGzip.Header.Set("Accept-Encoding", "gzip")
	respGzip, err := client.Do(reqGzip)
	require.NoError(t, err)
	assert.Equal(t, "gzip", respGzip.Header.Get("Content-Encoding"))

	gzCompressedBody, _ := io.ReadAll(respGzip.Body)
	_ = respGzip.Body.Close()
	gzDecompressed, err := intCompress.DecompressGzip(gzCompressedBody)
	require.NoError(t, err)

	var gzResult UserData
	require.NoError(t, json.Unmarshal(gzDecompressed, &gzResult))
	assert.Equal(t, largeText, gzResult.Payload)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestInboundBrotliRequestBodyDecompression(t *testing.T) {
	app := sein.New()

	type InputDTO struct {
		Message string `json:"message"`
	}

	app.Post("/echo", func(ctx context.Context, req InputDTO) (string, error) {
		return "Echo: " + req.Message, nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()

	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// Compress request body with Brotli
	rawJSON, _ := json.Marshal(InputDTO{Message: "Secret payload over Brotli"})
	compressedReqBody, err := intCompress.CompressBrotli(rawJSON, intCompress.BrotliDefaultCompression)
	require.NoError(t, err)

	postReq, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/echo", bytes.NewReader(compressedReqBody))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Content-Encoding", "br")

	resp, err := client.Do(postReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	assert.Equal(t, "Echo: Secret payload over Brotli", string(respBody))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
