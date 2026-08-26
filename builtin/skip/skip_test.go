// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package skip_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/skip"
)

func TestSkip_BypassCondition(t *testing.T) {
	app := sein.New()

	// A middleware that injects X-Custom-Header unless path starts with "/public"
	customMW := func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			res, err := next(req)
			if err != nil {
				return nil, err
			}
			return sein.OK[any](res).WithHeader("X-Custom-Header", "injected"), nil
		}
	}

	app.Use(skip.New(customMW, func(req *sein.Request) bool {
		return req.Path() == "/public"
	}))

	app.Get("/public", func(ctx context.Context) (string, error) {
		return "Public", nil
	})

	app.Get("/private", func(ctx context.Context) (string, error) {
		return "Private", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. GET /public -> middleware skipped, no X-Custom-Header
	resp1, err := client.Get("http://" + addr + "/public")
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)
	assert.Empty(t, resp1.Header.Get("X-Custom-Header"))

	// 2. GET /private -> middleware executed, X-Custom-Header present
	resp2, err := client.Get("http://" + addr + "/private")
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	assert.Equal(t, "injected", resp2.Header.Get("X-Custom-Header"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
