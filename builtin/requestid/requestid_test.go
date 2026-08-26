// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package requestid_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/requestid"
)

func TestRequestID_AutoGenerate(t *testing.T) {
	s := sein.New()
	s.Use(requestid.Default())

	var capturedID string
	s.Get("/id", func(ctx context.Context) (string, error) {
		capturedID = requestid.FromContext(ctx)
		return "ok", nil
	})

	req := httptest.NewRequest("GET", "/id", nil)
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	respID := w.Header().Get(header.XRequestID)
	require.NotEmpty(t, respID)
	assert.Equal(t, respID, capturedID)
}

func TestRequestID_PreserveIncoming(t *testing.T) {
	s := sein.New()
	s.Use(requestid.Default())

	var capturedID string
	s.Get("/id", func(ctx context.Context) (string, error) {
		capturedID = requestid.FromContext(ctx)
		return "ok", nil
	})

	req := httptest.NewRequest("GET", "/id", nil)
	incomingID := "custom-trace-uuid-12345"
	req.Header.Set(header.XRequestID, incomingID)

	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, incomingID, w.Header().Get(header.XRequestID))
	assert.Equal(t, incomingID, capturedID)
}
