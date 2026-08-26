// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package recover_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	recovermw "github.com/lemon4ksan/sein/builtin/recover"
)

func TestRecover_CatchesPanic(t *testing.T) {
	app := sein.New()
	app.Use(recovermw.New())

	app.Get("/panic", func(ctx context.Context) (string, error) {
		panic("simulated critical crash")
	})

	app.Get("/ok", func(ctx context.Context) (string, error) {
		return "all good", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Panicking route -> 500
	resp1, err := client.Get("http://" + addr + "/panic")
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	assert.Equal(t, http.StatusInternalServerError, resp1.StatusCode)

	// 2. Normal route -> 200
	resp2, err := client.Get("http://" + addr + "/ok")
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestRecover_CustomErrorHandler(t *testing.T) {
	app := sein.New()
	app.Use(recovermw.New(
		recovermw.WithStackTrace(false),
		recovermw.WithErrorHandler(func(req *sein.Request, err any) (any, error) {
			return nil, sein.ErrBadRequest("custom error message on panic")
		}),
	))

	app.Get("/panic-custom", func(ctx context.Context) (string, error) {
		panic("custom panic")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get("http://" + addr + "/panic-custom")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
