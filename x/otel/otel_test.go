// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package otel_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/x/otel"
)

type memoryExporter struct {
	mu    sync.Mutex
	spans []*otel.Span
}

func (m *memoryExporter) Export(spans []*otel.Span) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spans = append(m.spans, spans...)
}

func (m *memoryExporter) Shutdown(ctx context.Context) error {
	return nil
}

func TestOTel_TracingLifecycle(t *testing.T) {
	exporter := &memoryExporter{}

	app := sein.New()
	app.Use(otel.New(
		otel.WithServiceName("my-backend-service"),
		otel.WithExporter(exporter),
	))

	app.Get("/order/:id", func(ctx context.Context) (string, error) {
		span := otel.SpanFromContext(ctx)
		require.NotNil(t, span)
		span.SetAttribute("order.id", 999)

		return "order processed", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Send request with incoming W3C traceparent
	incomingTraceParent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/order/999", nil)
	req.Header.Set("traceparent", incomingTraceParent)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("traceparent"))
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", resp.Header.Get("X-Trace-ID"))

	// 2. Verify exported span
	require.Equal(t, 1, len(exporter.spans))
	exported := exporter.spans[0]
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", exported.TraceID)
	assert.Equal(t, "00f067aa0ba902b7", exported.ParentSpanID)
	assert.Equal(t, "GET /order/999", exported.Name)
	assert.Equal(t, "OK", exported.Status)
	assert.Equal(t, 999, exported.Attributes["order.id"])
	assert.Equal(t, "my-backend-service", exported.Attributes["service.name"])

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

func TestOTel_OTLPExporter_Filter_And_Errors(t *testing.T) {
	receivedCh := make(chan struct{}, 1)

	mockOTLP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/traces", r.URL.Path)
		select {
		case receivedCh <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockOTLP.Close()

	app := sein.New()
	app.Use(otel.New(
		otel.WithOTLPExporter(mockOTLP.URL),
		otel.WithFilter(func(req *sein.Request) bool {
			return req.Path() == "/health"
		}),
	))

	app.Get("/health", func(ctx context.Context) (string, error) {
		return "ok", nil
	})

	app.Get("/error-span", func(ctx context.Context) (string, error) {
		traceID := otel.TraceIDFromContext(ctx)
		assert.NotEmpty(t, traceID)
		return "", sein.ErrUnauthorized("missing-credentials")
	})

	// 1. Filtered route
	recHealth := httptest.NewRecorder()
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	app.ServeHTTP(recHealth, reqHealth)
	assert.Equal(t, http.StatusOK, recHealth.Code)
	assert.Empty(t, recHealth.Header().Get("traceparent"))

	// 2. Error route with trace context
	recErr := httptest.NewRecorder()
	reqErr := httptest.NewRequest(http.MethodGet, "/error-span", nil)
	app.ServeHTTP(recErr, reqErr)
	assert.Equal(t, http.StatusUnauthorized, recErr.Code)

	// Wait for OTLP exporter worker flush
	select {
	case <-receivedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for OTLP export")
	}
}
