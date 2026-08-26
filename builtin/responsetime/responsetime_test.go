// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package responsetime_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/responsetime"
)

func TestResponseTime_Headers(t *testing.T) {
	app := sein.New()
	app.Use(responsetime.New(
		responsetime.WithServerTiming(true),
	))

	app.Get("/work", func(ctx context.Context) (string, error) {
		time.Sleep(10 * time.Millisecond)
		return "Done", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get("http://" + addr + "/work")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get(responsetime.DefaultHeader))
	assert.Contains(t, resp.Header.Get(responsetime.DefaultHeader), "ms")
	assert.Contains(t, resp.Header.Get("Server-Timing"), "total;dur=")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
