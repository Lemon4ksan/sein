// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dump_test

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/dump"
)

func TestDump_RequestAndResponse(t *testing.T) {
	var (
		mu      sync.Mutex
		dumpOut string
	)

	app := sein.New()
	app.Use(dump.New(
		dump.WithOutput(func(d string) {
			mu.Lock()
			dumpOut = d
			mu.Unlock()
		}),
	))

	app.Post("/users", func(ctx context.Context) (string, error) {
		return "user created", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/users", strings.NewReader(`{"name":"bob"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	mu.Lock()
	captured := dumpOut
	mu.Unlock()

	assert.Contains(t, captured, "HTTP REQUEST")
	assert.Contains(t, captured, "POST /users")
	assert.Contains(t, captured, "curl -X POST")
	assert.Contains(t, captured, "HTTP RESPONSE")
	assert.Contains(t, captured, "user created")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
