// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package healthcheck_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/healthcheck"
)

func TestHealthcheck_LivezAndReadyz(t *testing.T) {
	var dbHealthy atomic.Bool
	dbHealthy.Store(true)

	app := sein.New()
	healthcheck.Register(app,
		healthcheck.WithLiveChecker("process", func(ctx context.Context) error {
			return nil
		}),
		healthcheck.WithReadyChecker("database", func(ctx context.Context) error {
			if !dbHealthy.Load() {
				return errors.New("connection refused")
			}
			return nil
		}),
	)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. GET /livez -> 200 OK
	respLive, err := client.Get("http://" + addr + "/livez")
	require.NoError(t, err)
	defer func() { _ = respLive.Body.Close() }()
	assert.Equal(t, http.StatusOK, respLive.StatusCode)

	bodyLive, _ := io.ReadAll(respLive.Body)
	assert.Contains(t, string(bodyLive), `"status":"UP"`)
	assert.Contains(t, string(bodyLive), `"process":"OK"`)

	// 2. GET /readyz healthy -> 200 OK
	respReady1, err := client.Get("http://" + addr + "/readyz")
	require.NoError(t, err)
	defer func() { _ = respReady1.Body.Close() }()
	assert.Equal(t, http.StatusOK, respReady1.StatusCode)

	bodyReady1, _ := io.ReadAll(respReady1.Body)
	assert.Contains(t, string(bodyReady1), `"status":"UP"`)
	assert.Contains(t, string(bodyReady1), `"database":"OK"`)

	// 3. Database fails -> GET /readyz returns 503 DOWN
	dbHealthy.Store(false)

	respReady2, err := client.Get("http://" + addr + "/readyz")
	require.NoError(t, err)
	defer func() { _ = respReady2.Body.Close() }()
	assert.Equal(t, http.StatusServiceUnavailable, respReady2.StatusCode)

	bodyReady2, _ := io.ReadAll(respReady2.Body)
	assert.Contains(t, string(bodyReady2), `"status":"DOWN"`)
	assert.Contains(t, string(bodyReady2), "connection refused")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestHealthcheck_Middleware_CustomPaths(t *testing.T) {
	app := sein.New()
	app.Use(healthcheck.New(
		healthcheck.WithLivenessPath("/health/live"),
		healthcheck.WithReadinessPath("/health/ready"),
		healthcheck.WithLiveChecker("cpu", func(ctx context.Context) error {
			return nil
		}),
	))

	app.Get("/data", func(ctx context.Context) (string, error) {
		return "data", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Custom liveness path
	respLive, err := client.Get("http://" + addr + "/health/live")
	require.NoError(t, err)
	defer func() { _ = respLive.Body.Close() }()
	assert.Equal(t, http.StatusOK, respLive.StatusCode)

	// 2. Custom readiness path
	respReady, err := client.Get("http://" + addr + "/health/ready")
	require.NoError(t, err)
	defer func() { _ = respReady.Body.Close() }()
	assert.Equal(t, http.StatusOK, respReady.StatusCode)

	// 3. Normal route passthrough
	respData, err := client.Get("http://" + addr + "/data")
	require.NoError(t, err)
	defer func() { _ = respData.Body.Close() }()
	assert.Equal(t, http.StatusOK, respData.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
