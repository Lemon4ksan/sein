// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipfilter_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/ipfilter"
)

func TestIPFilter_AllowAndBlock(t *testing.T) {
	// Whitelist 127.0.0.1, block 127.0.0.2
	app := sein.New()
	app.Use(ipfilter.New(
		ipfilter.WithAllow("127.0.0.1", "10.0.0.0/8"),
		ipfilter.WithBlock("127.0.0.2"),
	))

	app.Get("/admin", func(ctx context.Context) (string, error) {
		return "admin panel", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Connection from 127.0.0.1 -> Allowed (200 OK)
	resp, err := client.Get("http://" + addr + "/admin")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
