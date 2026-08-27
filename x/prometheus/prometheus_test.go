// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prometheus_test

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
	"github.com/lemon4ksan/sein/x/prometheus"
)

func TestPrometheus_MetricsGathering(t *testing.T) {
	app := sein.New()
	prometheus.Register(app)

	app.Get("/users/:id", func(ctx context.Context, id string) (string, error) {
		return "user " + id, nil
	})

	app.Get("/users", func(ctx context.Context) (string, error) {
		return "users list", nil
	})

	app.Get("/fail", func(ctx context.Context) (string, error) {
		return "", sein.ErrBadRequest("bad input")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Send some HTTP requests
	for range 3 {
		resp, err := client.Get("http://" + addr + "/users")
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}

	// Dynamic path parameters should group under /users/:id
	for _, id := range []string{"1001", "1002", "1003"} {
		resp, err := client.Get("http://" + addr + "/users/" + id)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}

	respFail, err := client.Get("http://" + addr + "/fail")
	require.NoError(t, err)
	_ = respFail.Body.Close()
	assert.Equal(t, http.StatusBadRequest, respFail.StatusCode)

	// 2. Fetch /metrics
	respMetrics, err := client.Get("http://" + addr + "/metrics")
	require.NoError(t, err)
	defer func() { _ = respMetrics.Body.Close() }()

	assert.Equal(t, http.StatusOK, respMetrics.StatusCode)
	body, err := io.ReadAll(respMetrics.Body)
	require.NoError(t, err)

	metricsText := string(body)
	assert.Contains(t, metricsText, "http_requests_total{method=\"GET\",path=\"/users\",status=\"200\"} 3")
	assert.Contains(t, metricsText, "http_requests_total{method=\"GET\",path=\"/users/:id\",status=\"200\"} 3")
	assert.Contains(t, metricsText, "http_requests_total{method=\"GET\",path=\"/fail\",status=\"400\"} 1")
	assert.Contains(t, metricsText, "go_goroutines")
	assert.Contains(t, metricsText, "process_resident_memory_bytes")
	assert.Contains(t, metricsText, "http_request_duration_seconds_bucket")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
