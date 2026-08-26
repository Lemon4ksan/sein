// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package circuitbreaker_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/circuitbreaker"
)

func TestCircuitBreaker_FailFastAndRecovery(t *testing.T) {
	var shouldFail atomic.Bool
	var handlerCallCount atomic.Int64

	app := sein.New()
	app.Use(circuitbreaker.New(
		circuitbreaker.WithFailureThreshold(0.5),
		circuitbreaker.WithMinRequests(4),
		circuitbreaker.WithCooldown(200*time.Millisecond),
		circuitbreaker.WithWindow(5*time.Second),
	))

	app.Get("/flaky-service", func(ctx context.Context) (string, error) {
		handlerCallCount.Add(1)
		if shouldFail.Load() {
			return "", errors.New("downstream database timeout")
		}

		return "success", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Initial healthy requests -> 200 OK
	for i := 0; i < 4; i++ {
		resp, err := client.Get("http://" + addr + "/flaky-service")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	}

	// 2. Enable failure mode and trigger circuit trip
	shouldFail.Store(true)
	for i := 0; i < 5; i++ {
		resp, err := client.Get("http://" + addr + "/flaky-service")
		require.NoError(t, err)
		_ = resp.Body.Close()
	}

	// 3. Circuit is now OPEN: next requests should be rejected with 503 WITHOUT reaching the handler
	callCountBefore := handlerCallCount.Load()
	resp503, err := client.Get("http://" + addr + "/flaky-service")
	require.NoError(t, err)
	defer func() { _ = resp503.Body.Close() }()
	assert.Equal(t, http.StatusServiceUnavailable, resp503.StatusCode)
	// Handler was NOT called!
	assert.Equal(t, callCountBefore, handlerCallCount.Load())

	// 4. Wait for Cooldown to transition to Half-Open and recover
	time.Sleep(300 * time.Millisecond)
	shouldFail.Store(false)

	respRecovered, err := client.Get("http://" + addr + "/flaky-service")
	require.NoError(t, err)
	defer func() { _ = respRecovered.Body.Close() }()
	assert.Equal(t, http.StatusOK, respRecovered.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
