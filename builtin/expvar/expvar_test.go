// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package expvar_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/expvar"
)

func TestExpvar_Endpoint(t *testing.T) {
	app := sein.New()
	expvar.Register(app)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get("http://" + addr + "/debug/vars")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Contains(t, string(body), "cmdline")
	assert.Contains(t, string(body), "memstats")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestExpvar_Middleware_CustomPath(t *testing.T) {
	app := sein.New()
	app.Use(expvar.New("/custom/metrics"))

	app.Get("/hello", func(ctx context.Context) (string, error) {
		return "world", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. /custom/metrics returns expvar JSON
	resp1, err := client.Get("http://" + addr + "/custom/metrics")
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)
	body1, _ := io.ReadAll(resp1.Body)
	assert.Contains(t, string(body1), "memstats")

	// 2. /hello passes through to normal route
	resp2, err := client.Get("http://" + addr + "/hello")
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
