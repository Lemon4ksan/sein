// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package helmet_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/helmet"
)

func TestHelmet_DefaultHeaders(t *testing.T) {
	app := sein.New()
	app.Use(helmet.New())

	app.Get("/test", func(ctx context.Context) (string, error) {
		return "Hello Helmet", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get("http://" + addr + "/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "nosniff", resp.Header.Get(header.XContentTypeOptions))
	assert.Equal(t, "SAMEORIGIN", resp.Header.Get(header.XFrameOptions))
	assert.Equal(t, "0", resp.Header.Get(header.XXSSProtection))
	assert.Contains(t, resp.Header.Get(header.StrictTransportSecurity), "max-age=31536000")
	assert.Equal(t, "no-referrer", resp.Header.Get(header.ReferrerPolicy))
	assert.Equal(t, "same-origin", resp.Header.Get(header.CrossOriginOpenerPolicy))
	assert.Equal(t, "same-origin", resp.Header.Get(header.CrossOriginResourcePolicy))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestHelmet_CustomCSPAndHSTS(t *testing.T) {
	app := sein.New()
	app.Use(helmet.New(
		helmet.WithCSP("default-src 'self'; script-src 'self' https://trusted.cdn.com"),
		helmet.WithHSTS(63072000, true, true),
		helmet.WithXFrameOptions("DENY"),
	))

	app.Get("/secure", func(ctx context.Context) (string, error) {
		return "Secure", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get("http://" + addr + "/secure")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, "DENY", resp.Header.Get(header.XFrameOptions))
	assert.Equal(t, "default-src 'self'; script-src 'self' https://trusted.cdn.com", resp.Header.Get(header.ContentSecurityPolicy))
	assert.Equal(t, "max-age=63072000; includeSubDomains; preload", resp.Header.Get(header.StrictTransportSecurity))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
