// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package earlydata_test

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
	"github.com/lemon4ksan/sein/builtin/earlydata"
)

func TestEarlyData_SafeAndUnsafeMethods(t *testing.T) {
	app := sein.New()
	app.Use(earlydata.New(
		earlydata.WithAllowPaths("/api/telemetry"),
	))

	app.Get("/data", func(ctx context.Context) (string, error) {
		return "GET Data", nil
	})

	app.Post("/orders", func(ctx context.Context, _ struct{}) (string, error) {
		return "Order Placed", nil
	})

	app.Post("/api/telemetry", func(ctx context.Context, _ struct{}) (string, error) {
		return "Telemetry Accepted", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Normal POST without Early-Data -> 200 OK
	reqNormal, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/orders", nil)
	respNormal, err := client.Do(reqNormal)
	require.NoError(t, err)
	defer func() { _ = respNormal.Body.Close() }()
	assert.Equal(t, http.StatusOK, respNormal.StatusCode)

	// 2. Safe GET with Early-Data: 1 -> 200 OK
	reqGetEarly, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/data", nil)
	reqGetEarly.Header.Set(earlydata.HeaderEarlyData, "1")
	respGetEarly, err := client.Do(reqGetEarly)
	require.NoError(t, err)
	defer func() { _ = respGetEarly.Body.Close() }()
	assert.Equal(t, http.StatusOK, respGetEarly.StatusCode)

	// 3. Mutating POST with Early-Data: 1 -> 425 Too Early
	reqPostEarly, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/orders", nil)
	reqPostEarly.Header.Set(earlydata.HeaderEarlyData, "1")
	respPostEarly, err := client.Do(reqPostEarly)
	require.NoError(t, err)
	defer func() { _ = respPostEarly.Body.Close() }()
	assert.Equal(t, http.StatusTooEarly, respPostEarly.StatusCode)

	// 4. Whitelisted Path POST with Early-Data: 1 -> 200 OK
	reqTelemetryEarly, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/api/telemetry", nil)
	reqTelemetryEarly.Header.Set(earlydata.HeaderEarlyData, "1")
	respTelemetryEarly, err := client.Do(reqTelemetryEarly)
	require.NoError(t, err)
	defer func() { _ = respTelemetryEarly.Body.Close() }()
	assert.Equal(t, http.StatusOK, respTelemetryEarly.StatusCode)
	body, _ := io.ReadAll(respTelemetryEarly.Body)
	assert.Equal(t, "Telemetry Accepted", string(body))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestEarlyData_CustomPredicate(t *testing.T) {
	app := sein.New()
	app.Use(earlydata.New(
		earlydata.WithAllowFunc(func(req *sein.Request) bool {
			return req.Header("X-Replay-Safe") == "true"
		}),
	))

	app.Post("/safe-mutation", func(ctx context.Context, _ struct{}) (string, error) {
		return "Mutation Done", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Without header -> 425
	req1, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/safe-mutation", nil)
	req1.Header.Set(earlydata.HeaderEarlyData, "1")
	resp1, err := client.Do(req1)
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	assert.Equal(t, http.StatusTooEarly, resp1.StatusCode)

	// 2. With X-Replay-Safe: true -> 200 OK
	req2, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/safe-mutation", nil)
	req2.Header.Set(earlydata.HeaderEarlyData, "1")
	req2.Header.Set("X-Replay-Safe", "true")
	resp2, err := client.Do(req2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
