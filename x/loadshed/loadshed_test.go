// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package loadshed_test

import (
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/x/loadshed"
)

func TestLoadShed_MaxInFlight(t *testing.T) {
	app := sein.New()
	app.Use(loadshed.New(
		loadshed.WithMaxInFlight(2),
	))

	blockCh := make(chan struct{})
	var entered atomic.Int64

	app.Get("/heavy", func(ctx context.Context) (string, error) {
		entered.Add(1)
		<-blockCh
		return "done", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	var wg sync.WaitGroup

	// Launch 2 slow requests (occupying slots)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get("http://" + addr + "/heavy")
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
	}

	// Wait until both slow requests are inside
	for entered.Load() < 2 {
		time.Sleep(10 * time.Millisecond)
	}

	// 3rd request should immediately be shed (503 Service Unavailable)
	resp3, err := client.Get("http://" + addr + "/heavy")
	require.NoError(t, err)
	defer func() { _ = resp3.Body.Close() }()
	assert.Equal(t, http.StatusServiceUnavailable, resp3.StatusCode)

	// Unblock slow requests
	close(blockCh)
	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
