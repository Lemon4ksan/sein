// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swaggerui_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/x/swaggerui"
)

func TestSwaggerUI_Endpoints(t *testing.T) {
	app := sein.New()

	specJSON := []byte(`{"openapi":"3.0.0","info":{"title":"Sample API","version":"1.0.0"}}`)

	swaggerui.Register(app,
		swaggerui.WithPath("/docs"),
		swaggerui.WithSpecURL("/docs/openapi.json"),
		swaggerui.WithSpecData(specJSON),
		swaggerui.WithTitle("My API Docs"),
	)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. GET /docs -> HTML
	respHTML, err := client.Get("http://" + addr + "/docs")
	require.NoError(t, err)
	defer func() { _ = respHTML.Body.Close() }()

	assert.Equal(t, http.StatusOK, respHTML.StatusCode)
	bodyHTML, _ := io.ReadAll(respHTML.Body)
	assert.Contains(t, string(bodyHTML), "My API Docs")
	assert.Contains(t, string(bodyHTML), "/docs/openapi.json")

	// 2. GET /docs/openapi.json -> JSON spec
	respSpec, err := client.Get("http://" + addr + "/docs/openapi.json")
	require.NoError(t, err)
	defer func() { _ = respSpec.Body.Close() }()

	assert.Equal(t, http.StatusOK, respSpec.StatusCode)
	bodySpec, _ := io.ReadAll(respSpec.Body)
	assert.Equal(t, string(specJSON), string(bodySpec))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
