// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package preset_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein/preset"
)

func TestPreset_Production(t *testing.T) {
	app := preset.Production(
		preset.WithPrometheus("/metrics"),
		preset.WithRevision("1.2.3", "/version"),
		preset.WithCORS(preset.CORSConfig{
			AllowOrigins: []string{"*"},
		}),
	)

	app.Get("/health", func(ctx context.Context) (string, error) {
		return "ok", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Health check with security headers & response time
	resp, err := client.Get("http://" + addr + "/health")
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("X-Response-Time"))
	assert.Equal(t, "1.2.3", resp.Header.Get("X-App-Version"))

	// 2. Metrics endpoint
	respMet, err := client.Get("http://" + addr + "/metrics")
	require.NoError(t, err)
	_ = respMet.Body.Close()
	assert.Equal(t, http.StatusOK, respMet.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
