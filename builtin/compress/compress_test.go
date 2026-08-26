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
	"github.com/lemon4ksan/sein/builtin/compress"
	intCompress "github.com/lemon4ksan/sein/internal/compress"
)

func TestResponseCompression_ZstdBrotliGzip(t *testing.T) {
	app := sein.New()
	app.Use(compress.New(compress.WithMinLength(100)))

	largeText := strings.Repeat("Zstd, Brotli, and Gzip multi-algorithm server compression! ", 30)

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

	// 1. Request with Accept-Encoding: zstd (should pick Zstd)
	reqZstd, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/data", nil)
	reqZstd.Header.Set("Accept-Encoding", "zstd, br, gzip")
	respZstd, err := client.Do(reqZstd)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respZstd.StatusCode)
	assert.Equal(t, "zstd", respZstd.Header.Get("Content-Encoding"))

	zstdCompressed, _ := io.ReadAll(respZstd.Body)
	_ = respZstd.Body.Close()
	zstdDecompressed, err := intCompress.DecompressZstd(zstdCompressed)
	require.NoError(t, err)

	var zstdResult UserData
	require.NoError(t, json.Unmarshal(zstdDecompressed, &zstdResult))
	assert.Equal(t, largeText, zstdResult.Payload)

	// 2. Request with Accept-Encoding: br (should pick Brotli)
	reqBr, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/data", nil)
	reqBr.Header.Set("Accept-Encoding", "br, gzip")
	respBr, err := client.Do(reqBr)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respBr.StatusCode)
	assert.Equal(t, "br", respBr.Header.Get("Content-Encoding"))

	brCompressed, _ := io.ReadAll(respBr.Body)
	_ = respBr.Body.Close()
	brDecompressed, err := intCompress.DecompressBrotli(brCompressed)
	require.NoError(t, err)

	var brResult UserData
	require.NoError(t, json.Unmarshal(brDecompressed, &brResult))
	assert.Equal(t, largeText, brResult.Payload)

	// 3. Request with Accept-Encoding: gzip (should pick Gzip)
	reqGzip, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/data", nil)
	reqGzip.Header.Set("Accept-Encoding", "gzip")
	respGzip, err := client.Do(reqGzip)
	require.NoError(t, err)
	assert.Equal(t, "gzip", respGzip.Header.Get("Content-Encoding"))

	gzCompressed, _ := io.ReadAll(respGzip.Body)
	_ = respGzip.Body.Close()
	gzDecompressed, err := intCompress.DecompressGzip(gzCompressed)
	require.NoError(t, err)

	var gzResult UserData
	require.NoError(t, json.Unmarshal(gzDecompressed, &gzResult))
	assert.Equal(t, largeText, gzResult.Payload)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, app.Shutdown(ctx))
}

func TestInboundCompressedRequestBodyDecompression(t *testing.T) {
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

	// 1. Zstd Request Body
	rawJSON, _ := json.Marshal(InputDTO{Message: "Secret Zstd payload"})
	zstdComp, err := intCompress.CompressZstd(rawJSON, intCompress.ZstdSpeedDefault)
	require.NoError(t, err)

	postZstd, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/echo", bytes.NewReader(zstdComp))
	postZstd.Header.Set("Content-Type", "application/json")
	postZstd.Header.Set("Content-Encoding", "zstd")

	respZstd, err := client.Do(postZstd)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respZstd.StatusCode)
	zstdEcho, _ := io.ReadAll(respZstd.Body)
	_ = respZstd.Body.Close()

	assert.Equal(t, "Echo: Secret Zstd payload", string(zstdEcho))

	// 2. Brotli Request Body
	brJSON, _ := json.Marshal(InputDTO{Message: "Secret Brotli payload"})
	brComp, err := intCompress.CompressBrotli(brJSON, intCompress.BrotliDefaultCompression)
	require.NoError(t, err)

	postBr, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/echo", bytes.NewReader(brComp))
	postBr.Header.Set("Content-Type", "application/json")
	postBr.Header.Set("Content-Encoding", "br")

	respBr, err := client.Do(postBr)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respBr.StatusCode)
	brEcho, _ := io.ReadAll(respBr.Body)
	_ = respBr.Body.Close()

	assert.Equal(t, "Echo: Secret Brotli payload", string(brEcho))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, app.Shutdown(ctx))
}
