// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package timeout_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/timeout"
)

func TestTimeout_Enforcement(t *testing.T) {
	app := sein.New()
	app.Use(timeout.New(
		timeout.WithTimeout(300 * time.Millisecond),
	))

	app.Get("/fast", func(ctx context.Context) (string, error) {
		return "fast response", nil
	})

	app.Get("/slow", func(ctx context.Context) (string, error) {
		time.Sleep(800 * time.Millisecond)
		return "too slow", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. GET /fast -> 200 OK
	resp1, err := client.Get("http://" + addr + "/fast")
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	// 2. GET /slow -> 504 Gateway Timeout
	resp2, err := client.Get("http://" + addr + "/slow")
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusGatewayTimeout, resp2.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
