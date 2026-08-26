// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package idempotency_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/idempotency"
)

func TestIdempotency_ExecutionAndReplay(t *testing.T) {
	app := sein.New()
	app.Use(idempotency.New(
		idempotency.WithLifetime(1 * time.Hour),
	))

	var executionCount atomic.Int64

	type PaymentResult struct {
		TxID  string `json:"tx_id"`
		Count int64  `json:"count"`
	}

	app.Post("/pay", func(ctx context.Context, _ struct{}) (PaymentResult, error) {
		cnt := executionCount.Add(1)
		return PaymentResult{
			TxID:  "TX-9988-ABC",
			Count: cnt,
		}, nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Initial Request with Idempotency-Key
	req1, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/pay", nil)
	req1.Header.Set(idempotency.HeaderIdempotencyKey, "idemp-key-100")
	resp1, err := client.Do(req1)
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp1.StatusCode)
	body1, _ := io.ReadAll(resp1.Body)
	assert.Contains(t, string(body1), `"count":1`)
	assert.Equal(t, int64(1), executionCount.Load())

	// 2. Replay identical request with same Idempotency-Key
	req2, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/pay", nil)
	req2.Header.Set(idempotency.HeaderIdempotencyKey, "idemp-key-100")
	resp2, err := client.Do(req2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	assert.Equal(t, "true", resp2.Header.Get("Idempotent-Replayed"))
	body2, _ := io.ReadAll(resp2.Body)
	// Body should match first response exactly, and handler should NOT have re-executed
	assert.Equal(t, string(body1), string(body2))
	assert.Equal(t, int64(1), executionCount.Load())

	// 3. New request with different Idempotency-Key
	req3, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/pay", nil)
	req3.Header.Set(idempotency.HeaderIdempotencyKey, "idemp-key-200")
	resp3, err := client.Do(req3)
	require.NoError(t, err)
	defer func() { _ = resp3.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp3.StatusCode)
	assert.Equal(t, "", resp3.Header.Get("Idempotent-Replayed"))
	assert.Equal(t, int64(2), executionCount.Load())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestIdempotency_InFlightConflict(t *testing.T) {
	app := sein.New()
	app.Use(idempotency.New(
		idempotency.WithLockTimeout(5 * time.Second),
	))

	started := make(chan struct{})
	finish := make(chan struct{})

	app.Post("/slow-work", func(ctx context.Context, _ struct{}) (string, error) {
		close(started)
		<-finish
		return "Done", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 10 * time.Second}

	go func() {
		req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/slow-work", nil)
		req.Header.Set(idempotency.HeaderIdempotencyKey, "key-concurrent-1")
		resp, _ := client.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()

	// Wait until handler is actively running
	<-started

	// Second request with same key while first is in-flight -> 409 Conflict
	req2, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/slow-work", nil)
	req2.Header.Set(idempotency.HeaderIdempotencyKey, "key-concurrent-1")
	resp2, err := client.Do(req2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()

	assert.Equal(t, http.StatusConflict, resp2.StatusCode)

	close(finish)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
