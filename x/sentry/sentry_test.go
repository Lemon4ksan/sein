// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sentry_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/x/sentry"
)

type memoryTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (m *memoryTransport) Send(event *sentry.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *memoryTransport) Flush(ctx context.Context) error {
	return nil
}

func TestSentry_ErrorAndPanicCapture(t *testing.T) {
	transport := &memoryTransport{}

	app := sein.New()
	app.Use(sentry.New(
		sentry.WithTransport(transport),
		sentry.WithEnvironment("staging"),
		sentry.WithRelease("v1.2.3"),
	))

	app.Get("/error", func(ctx context.Context) (string, error) {
		return "", sein.ErrBadRequest("invalid database entity")
	})

	app.Get("/panic", func(ctx context.Context) (string, error) {
		panic("nil pointer dereference simulation")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Error request
	resp1, err := client.Get("http://" + addr + "/error")
	require.NoError(t, err)
	_ = resp1.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp1.StatusCode)

	// 2. Panic request
	resp2, err := client.Get("http://" + addr + "/panic")
	require.NoError(t, err)
	_ = resp2.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp2.StatusCode)

	// 3. Verify captured events
	require.Equal(t, 2, len(transport.events))

	errEvent := transport.events[0]
	assert.Equal(t, "error", errEvent.Level)
	assert.Contains(t, errEvent.Message, "invalid database entity")
	assert.Equal(t, "staging", errEvent.Tags["environment"])

	panicEvent := transport.events[1]
	assert.Equal(t, "fatal", panicEvent.Level)
	assert.Contains(t, panicEvent.Message, "nil pointer dereference")
	assert.NotEmpty(t, panicEvent.StackTrace)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestSentry_HTTPTransport_And_Flush(t *testing.T) {
	var (
		receivedMu sync.Mutex
		received   []*sentry.Event
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.Header.Get("X-Sentry-Auth"), "sentry_key=testkey")
		var ev sentry.Event
		_ = json.NewDecoder(r.Body).Decode(&ev)
		receivedMu.Lock()
		received = append(received, &ev)
		receivedMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)

	dsn := "http://testkey@" + u.Host + "/42"

	app := sein.New()
	app.Use(sentry.New(
		sentry.WithDSN(dsn),
		sentry.WithEnvironment("test"),
	))

	app.Get("/fail", func(ctx context.Context) (string, error) {
		return "", sein.ErrBadRequest("failure-sample")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Wait briefly for worker ingestion
	time.Sleep(100 * time.Millisecond)

	receivedMu.Lock()
	defer receivedMu.Unlock()
	assert.NotEmpty(t, received)
	if len(received) > 0 {
		assert.Equal(t, "error", received[0].Level)
		assert.Contains(t, received[0].Message, "failure-sample")
	}
}
