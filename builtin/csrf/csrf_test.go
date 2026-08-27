// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package csrf_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/csrf"
)

func TestCSRF_Workflow(t *testing.T) {
	app := sein.New()
	app.Use(csrf.New())

	app.Get("/login", func(ctx context.Context) (string, error) {
		return "Login Page", nil
	})

	app.Post("/transfer", func(ctx context.Context, _ struct{}) (string, error) {
		return "Transfer Complete", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	client := &http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
	}

	// 1. GET request receives CSRF token cookie
	respGet, err := client.Get("http://" + addr + "/login")
	require.NoError(t, err)
	defer func() { _ = respGet.Body.Close() }()

	assert.Equal(t, http.StatusOK, respGet.StatusCode)

	baseURL, _ := url.Parse("http://" + addr)
	cookies := jar.Cookies(baseURL)
	var csrfToken string
	for _, c := range cookies {
		if c.Name == csrf.DefaultCookieName {
			csrfToken = c.Value
		}
	}
	assert.NotEmpty(t, csrfToken)

	// 2. POST without token -> 403 Forbidden
	reqNoToken, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/transfer", nil)
	respNoToken, err := client.Do(reqNoToken)
	require.NoError(t, err)
	defer func() { _ = respNoToken.Body.Close() }()
	assert.Equal(t, http.StatusForbidden, respNoToken.StatusCode)

	// 3. POST with invalid token -> 403 Forbidden
	reqBadToken, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/transfer", nil)
	reqBadToken.Header.Set(csrf.DefaultHeaderName, "invalid-token-1234")
	respBadToken, err := client.Do(reqBadToken)
	require.NoError(t, err)
	defer func() { _ = respBadToken.Body.Close() }()
	assert.Equal(t, http.StatusForbidden, respBadToken.StatusCode)

	// 4. POST with valid header token -> 200 OK
	reqValid, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/transfer", nil)
	reqValid.Header.Set(csrf.DefaultHeaderName, csrfToken)
	respValid, err := client.Do(reqValid)
	require.NoError(t, err)
	defer func() { _ = respValid.Body.Close() }()
	assert.Equal(t, http.StatusOK, respValid.StatusCode)
	body, _ := io.ReadAll(respValid.Body)
	assert.Equal(t, "Transfer Complete", string(body))

	// 5. POST with valid form field token -> 200 OK
	formBody := strings.NewReader("_csrf=" + csrfToken)
	reqForm, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/transfer", formBody)
	reqForm.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	respForm, err := client.Do(reqForm)
	require.NoError(t, err)
	defer func() { _ = respForm.Body.Close() }()
	assert.Equal(t, http.StatusOK, respForm.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestCSRF_CustomOptions_And_ErrorHandler(t *testing.T) {
	app := sein.New()
	app.Use(csrf.New(
		csrf.WithCookieName("custom_csrf_cookie"),
		csrf.WithHeaderName("X-Custom-CSRF"),
		csrf.WithFormField("custom_csrf_field"),
		csrf.WithCookieSecure(true),
		csrf.WithCookieHTTPOnly(true),
		csrf.WithCookieSameSite(http.SameSiteStrictMode),
		csrf.WithCookieDomain("example.com"),
		csrf.WithCookiePath("/api"),
		csrf.WithExpiration(12*time.Hour),
		csrf.WithErrorHandler(func(req *sein.Request) (any, error) {
			return sein.StatusWith[any](http.StatusTeapot, "custom csrf rejection", nil), nil
		}),
	))

	app.Post("/api/action", func(ctx context.Context) (string, error) {
		return "action done", nil
	})

	// Missing CSRF with custom error handler -> 418 Teapot
	req := httptest.NewRequest(http.MethodPost, "/api/action", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTeapot, rec.Code)
	assert.Contains(t, rec.Body.String(), "custom csrf rejection")
}
