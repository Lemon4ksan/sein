// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package revision_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/revision"
)

func TestRevision_HeaderAndEndpoint(t *testing.T) {
	app := sein.New()
	revision.Register(app,
		revision.WithVersion("2.4.0"),
		revision.WithCommit("a1b2c3d4"),
		revision.WithBuildTime("2026-08-26T18:00:00Z"),
	)

	app.Get("/ping", func(ctx context.Context) (string, error) {
		return "pong", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. GET /ping -> check response headers
	resp, err := client.Get("http://" + addr + "/ping")
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "2.4.0", resp.Header.Get("X-App-Version"))
	assert.Equal(t, "a1b2c3d4", resp.Header.Get("X-Git-Commit"))

	// 2. GET /version -> check JSON output
	respVer, err := client.Get("http://" + addr + "/version")
	require.NoError(t, err)
	defer func() { _ = respVer.Body.Close() }()

	assert.Equal(t, http.StatusOK, respVer.StatusCode)
	body, _ := io.ReadAll(respVer.Body)

	var info revision.Info
	require.NoError(t, json.Unmarshal(body, &info))
	assert.Equal(t, "2.4.0", info.Version)
	assert.Equal(t, "a1b2c3d4", info.Commit)
	assert.Equal(t, "2026-08-26T18:00:00Z", info.BuildTime)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
