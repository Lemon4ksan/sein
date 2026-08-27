// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sse_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/sse"
)

func TestSSE_StreamingEvents(t *testing.T) {
	s := sein.New()

	s.Get("/stream", func(ctx context.Context) (any, error) {
		return sse.Stream(func(w *sse.Writer) error {
			_ = w.SendComment("connected")
			_ = w.Send("hello initial")
			_ = w.SendEvent("chunk", map[string]string{"delta": "world"})
			_ = w.SendID("msg-42", "done")
			_ = w.SendRetry(5 * time.Second)

			return nil
		}), nil
	})

	req := httptest.NewRequest("GET", "/stream", nil)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))

	body := w.Body.String()
	require.True(t, strings.Contains(body, ": connected\n\n"))
	require.True(t, strings.Contains(body, "data: hello initial\n\n"))
	require.True(t, strings.Contains(body, "event: chunk\ndata: {\"delta\":\"world\"}\n\n"))
	require.True(t, strings.Contains(body, "id: msg-42\ndata: done\n\n"))
	require.True(t, strings.Contains(body, "retry: 5000\n\n"))
}

type StreamQuery struct {
	Channel string `query:"channel,required"`
}

func TestSSE_Handle_WithDTO(t *testing.T) {
	s := sein.New()

	sse.Handle(s, "/events", func(ctx context.Context, w *sse.Writer, q StreamQuery) error {
		_ = w.Send("channel=" + q.Channel)
		return nil
	})

	t.Run("Missing required query param returns 400 Bad Request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/events", nil)
		rec := httptest.NewRecorder()

		s.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Valid query param streams events", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/events?channel=news", nil)
		rec := httptest.NewRecorder()

		s.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
		assert.Contains(t, rec.Body.String(), "data: channel=news\n\n")
	})
}

func TestSSE_AllHandlerSignatures_And_Multiline(t *testing.T) {
	app := sein.New()

	// 1. func(w *sse.Writer) error
	sse.Handle(app, "/sse1", func(w *sse.Writer) error {
		_ = w.Send("line1\nline2")
		_ = w.Send([]byte("byte1\nbyte2"))
		return nil
	})

	// 2. func(ctx context.Context, w *sse.Writer) error
	sse.Handle(app, "/sse2", func(ctx context.Context, w *sse.Writer) error {
		_ = w.Send("from-ctx")
		return nil
	})

	// 3. func(w *sse.Writer, opt DTO) error
	sse.Handle(app, "/sse3", func(w *sse.Writer, q StreamQuery) error {
		_ = w.Send("stream3=" + q.Channel)
		return nil
	})

	// 1. Test /sse1 multiline
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/sse1", nil)
	app.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Contains(t, rec1.Body.String(), "data: line1\ndata: line2\n\n")
	assert.Contains(t, rec1.Body.String(), "data: byte1\ndata: byte2\n\n")

	// 2. Test /sse2
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/sse2", nil)
	app.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "data: from-ctx\n\n")

	// 3. Test /sse3
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/sse3?channel=sports", nil)
	app.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusOK, rec3.Code)
	assert.Contains(t, rec3.Body.String(), "data: stream3=sports\n\n")

	// Test Response methods
	resp := sse.Stream(func(w *sse.Writer) error { return nil })
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	assert.Nil(t, resp.ResponseBody())
	assert.Nil(t, resp.ResponseCookies())
	assert.NotEmpty(t, resp.ResponseHeaders().Get("Content-Type"))
}
