// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package proxy_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/proxy"
)

func TestProxy_Forwarding(t *testing.T) {
	// 1. Backend upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/resource", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get(header.XForwardedFor))
		w.Header().Set("X-Upstream-Header", "upstream-value")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("backend payload"))
	}))
	defer upstream.Close()

	// 2. Gateway sein app with Proxy middleware
	app := sein.New()
	app.Use(proxy.New(
		proxy.WithTargets(upstream.URL),
	))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get("http://" + addr + "/api/v1/resource")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, "upstream-value", resp.Header.Get("X-Upstream-Header"))

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "backend payload", string(body))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
