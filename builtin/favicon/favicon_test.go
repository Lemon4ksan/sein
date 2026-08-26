// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package favicon_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/favicon"
)

func TestFavicon_Default204(t *testing.T) {
	app := sein.New()
	favicon.Register(app)

	app.Get("/home", func(ctx context.Context) (string, error) {
		return "Welcome", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Favicon -> 204 No Content
	resp, err := client.Get("http://" + addr + "/favicon.ico")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Contains(t, resp.Header.Get(header.CacheControl), "max-age=31536000")

	// 2. Normal path -> 200 OK
	respHome, err := client.Get("http://" + addr + "/home")
	require.NoError(t, err)
	defer func() { _ = respHome.Body.Close() }()
	assert.Equal(t, http.StatusOK, respHome.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestFavicon_InMemoryData(t *testing.T) {
	fakeIcon := []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10}

	app := sein.New()
	favicon.Register(app,
		favicon.WithData(fakeIcon),
	)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get("http://" + addr + "/favicon.ico")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "image/x-icon", resp.Header.Get(header.ContentType))

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, fakeIcon, body)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
