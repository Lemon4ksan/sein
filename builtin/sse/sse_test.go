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
