// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package methodoverride_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/methodoverride"
)

func TestMethodOverride_HeaderAndQuery(t *testing.T) {
	app := sein.New()
	app.Use(methodoverride.New())

	app.Delete("/resource/:id", func(ctx context.Context) (string, error) {
		return "deleted successfully", nil
	})

	app.Put("/resource/:id", func(ctx context.Context) (string, error) {
		return "updated successfully", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. POST /resource/42 with X-HTTP-Method-Override: DELETE
	req1, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/resource/42", strings.NewReader(""))
	req1.Header.Set("X-HTTP-Method-Override", "DELETE")
	resp1, err := client.Do(req1)
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp1.StatusCode)
	body1, _ := io.ReadAll(resp1.Body)
	assert.Equal(t, "deleted successfully", string(body1))

	// 2. POST /resource/42?_method=PUT
	req2, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/resource/42?_method=PUT", strings.NewReader(""))
	resp2, err := client.Do(req2)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	body2, _ := io.ReadAll(resp2.Body)
	assert.Equal(t, "updated successfully", string(body2))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
