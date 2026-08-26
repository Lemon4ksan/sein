// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package session_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/session"
)

func TestSession_LifecycleAndFlash(t *testing.T) {
	app := sein.New()
	app.Use(session.New())

	// Route 1: Login (set session & flash)
	app.GetReq("/login", func(req *sein.Request) (string, error) {
		sess := session.From(req)
		require.NotNil(t, sess)
		sess.Set("user_id", 1001)
		sess.Set("username", "alice")
		sess.Flash("notice", "Welcome back alice!")

		return "Logged in", nil
	})

	// Route 2: Dashboard (reads session & flash)
	app.GetReq("/dashboard", func(req *sein.Request) (string, error) {
		sess := session.From(req)
		require.NotNil(t, sess)

		userID := sess.GetInt("user_id")
		username := sess.GetString("username")
		flash := sess.Flash("notice")

		flashStr := ""
		if flash != nil {
			flashStr = flash.(string)
		}

		return "User: " + username + " (ID: " + string(rune('0'+userID%10)) + ") Flash: " + flashStr, nil
	})

	// Route 3: Logout (destroy session)
	app.GetReq("/logout", func(req *sein.Request) (string, error) {
		sess := session.From(req)
		require.NotNil(t, sess)
		_ = sess.Destroy()

		return "Logged out", nil
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

	// 1. GET /login
	resp1, err := client.Get("http://" + addr + "/login")
	require.NoError(t, err)
	_ = resp1.Body.Close()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	// 2. GET /dashboard (first time -> flash present)
	resp2, err := client.Get("http://" + addr + "/dashboard")
	require.NoError(t, err)
	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()
	assert.Contains(t, string(body2), "User: alice")
	assert.Contains(t, string(body2), "Flash: Welcome back alice!")

	// 3. GET /dashboard (second time -> flash consumed/empty)
	resp3, err := client.Get("http://" + addr + "/dashboard")
	require.NoError(t, err)
	body3, _ := io.ReadAll(resp3.Body)
	_ = resp3.Body.Close()
	assert.Contains(t, string(body3), "User: alice")
	assert.Contains(t, string(body3), "Flash: ")
	assert.NotContains(t, string(body3), "Welcome back alice!")

	// 4. GET /logout
	resp4, err := client.Get("http://" + addr + "/logout")
	require.NoError(t, err)
	_ = resp4.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
