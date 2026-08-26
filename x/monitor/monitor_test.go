// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package monitor_test

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
	"github.com/lemon4ksan/sein/x/monitor"
)

func TestMonitor_Endpoints(t *testing.T) {
	app := sein.New()
	monitor.Register(app,
		monitor.WithPath("/system-status"),
		monitor.WithTitle("Cluster Status"),
	)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. GET /system-status -> HTML Dashboard
	respHTML, err := client.Get("http://" + addr + "/system-status")
	require.NoError(t, err)
	defer func() { _ = respHTML.Body.Close() }()

	assert.Equal(t, http.StatusOK, respHTML.StatusCode)
	bodyHTML, _ := io.ReadAll(respHTML.Body)
	assert.Contains(t, string(bodyHTML), "Cluster Status")
	assert.Contains(t, string(bodyHTML), "/system-status/data")

	// 2. GET /system-status/data -> JSON metrics
	respData, err := client.Get("http://" + addr + "/system-status/data")
	require.NoError(t, err)
	defer func() { _ = respData.Body.Close() }()

	assert.Equal(t, http.StatusOK, respData.StatusCode)
	bodyData, _ := io.ReadAll(respData.Body)
	assert.Contains(t, string(bodyData), "goroutines")
	assert.Contains(t, string(bodyData), "alloc_mb")
	assert.Contains(t, string(bodyData), "num_cpu")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
