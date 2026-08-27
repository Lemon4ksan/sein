// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package logger_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/async/logkit"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	loggermw "github.com/lemon4ksan/sein/builtin/logger"
)

func TestLogger_AccessLogEmission(t *testing.T) {
	var buf bytes.Buffer
	cfg := logkit.DefaultConfig(logkit.LevelInfo)
	cfg.Output = &buf
	cfg.Colors = false
	customLogger := logkit.New(cfg)

	app := sein.New()
	app.Use(loggermw.New(
		loggermw.WithLogger(customLogger),
		loggermw.WithIgnorePaths("/health"),
	))

	app.Get("/users", func(ctx context.Context) (string, error) {
		return "user list", nil
	})

	app.Get("/health", func(ctx context.Context) (string, error) {
		return "ok", nil
	})

	app.Get("/error", func(ctx context.Context) (string, error) {
		return "", sein.ErrBadRequest("invalid parameter")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. GET /users -> 200 OK
	resp1, err := client.Get("http://" + addr + "/users")
	require.NoError(t, err)
	_ = resp1.Body.Close()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	// 2. GET /error -> 400 Bad Request
	resp2, err := client.Get("http://" + addr + "/error")
	require.NoError(t, err)
	_ = resp2.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp2.StatusCode)

	// 3. GET /health (ignored)
	resp3, err := client.Get("http://" + addr + "/health")
	require.NoError(t, err)
	_ = resp3.Body.Close()

	// Flush logger buffer
	time.Sleep(100 * time.Millisecond)
	_ = customLogger.Close()

	logs := buf.String()
	assert.Contains(t, logs, "path=/users")
	assert.Contains(t, logs, "path=/error")
	assert.NotContains(t, logs, "path=/health")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
