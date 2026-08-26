// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etag_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/etag"
)

func TestETag_Lifecycle(t *testing.T) {
	app := sein.New()
	app.Use(etag.New())

	app.Get("/resource", func(ctx context.Context) (string, error) {
		return "static resource payload 123", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Initial GET -> 200 OK with ETag header
	resp1, err := client.Get("http://" + addr + "/resource")
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp1.StatusCode)
	etagVal := resp1.Header.Get(header.ETag)
	assert.NotEmpty(t, etagVal)

	// 2. Second GET with If-None-Match -> 304 Not Modified
	req2, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/resource", nil)
	req2.Header.Set(header.IfNoneMatch, etagVal)

	resp2, err := client.Do(req2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()

	assert.Equal(t, http.StatusNotModified, resp2.StatusCode)
	body, _ := io.ReadAll(resp2.Body)
	assert.Empty(t, body)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
