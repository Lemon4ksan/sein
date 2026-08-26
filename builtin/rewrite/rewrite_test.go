// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rewrite_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/rewrite"
)

func TestRewrite_Rules(t *testing.T) {
	app := sein.New()
	app.Use(rewrite.New(
		rewrite.WithRule("/old-endpoint", "/new-endpoint"),
		rewrite.WithRule("/legacy/*", "/v2/$1"),
		rewrite.WithRegexRule(`^/user/(\d+)$`, "/api/user/$1"),
	))

	app.Get("/new-endpoint", func(ctx context.Context) (string, error) {
		return "rewritten exact", nil
	})

	app.Get("/v2/info", func(ctx context.Context) (string, error) {
		return "rewritten wildcard", nil
	})

	app.Get("/api/user/42", func(ctx context.Context) (string, error) {
		return "rewritten regex", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. GET /old-endpoint -> /new-endpoint
	resp1, err := client.Get("http://" + addr + "/old-endpoint")
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	// 2. GET /legacy/info -> /v2/info
	resp2, err := client.Get("http://" + addr + "/legacy/info")
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// 3. GET /user/42 -> /api/user/42
	resp3, err := client.Get("http://" + addr + "/user/42")
	require.NoError(t, err)
	defer func() { _ = resp3.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp3.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
