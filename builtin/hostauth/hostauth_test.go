// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hostauth_test

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
	"github.com/lemon4ksan/sein/builtin/hostauth"
)

func TestHostAuth_AllowedHosts(t *testing.T) {
	app := sein.New()
	app.Use(hostauth.New(
		hostauth.WithHosts(
			"example.com",
			"*.acme.corp",
			"localhost:*",
			"127.0.0.1:8080",
			"[::1]:*",
		),
	))

	app.Get("/hello", func(ctx context.Context) (string, error) {
		return "Authorized", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	tests := []struct {
		name       string
		hostHeader string
		wantStatus int
	}{
		{
			name:       "Exact Host match",
			hostHeader: "example.com",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Wildcard Subdomain 1 level",
			hostHeader: "api.acme.corp",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Wildcard Subdomain multi level",
			hostHeader: "stage.api.acme.corp",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Wildcard Base domain",
			hostHeader: "acme.corp",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Localhost with port wildcard",
			hostHeader: "localhost:3000",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Exact IP and Port match",
			hostHeader: "127.0.0.1:8080",
			wantStatus: http.StatusOK,
		},
		{
			name:       "IPv6 Literal with port wildcard",
			hostHeader: "[::1]:9000",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Unauthorized Host",
			hostHeader: "evil-attacker.com",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "Unauthorized Subdomain on exact domain",
			hostHeader: "sub.example.com",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "Unauthorized Port on exact IP",
			hostHeader: "127.0.0.1:9090",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "RFC 1035 Label Over 63 chars",
			hostHeader: strings.Repeat("a", 64) + ".com",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "http://"+addr+"/hello", nil)
			require.NoError(t, err)

			req.Host = tc.hostHeader

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tc.wantStatus, resp.StatusCode)

			if tc.wantStatus == http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				assert.Equal(t, "Authorized", string(body))
			}
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestHostAuth_CustomErrorHandler(t *testing.T) {
	app := sein.New()
	app.Use(hostauth.New(
		hostauth.WithHosts("trusted.internal"),
		hostauth.WithErrorHandler(func(req *sein.Request) (any, error) {
			return nil, sein.ErrBadRequest("host header is rejected by security policy")
		}),
	))

	app.Get("/data", func(ctx context.Context) (string, error) {
		return "secret data", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest(http.MethodGet, "http://"+addr+"/data", nil)
	require.NoError(t, err)
	req.Host = "untrusted.org"

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
