// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package static_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/static"
)

func TestStatic_EmbeddedFS_And_SPA(t *testing.T) {
	memFS := fstest.MapFS{
		"index.html":       {Data: []byte("<h1>Welcome to Sein</h1>")},
		"assets/style.css": {Data: []byte("body { background: #000; }")},
	}

	s := sein.New()
	handler := static.NewFS(memFS, static.Config{
		SPA: true,
	})

	s.MountRaw("GET", "/...", handler)

	// 1. Fetch index.html directly
	req1 := httptest.NewRequest("GET", "/index.html", nil)
	w1 := httptest.NewRecorder()
	s.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "<h1>Welcome to Sein</h1>", w1.Body.String())
	assert.Equal(t, "text/html; charset=utf-8", w1.Header().Get(header.ContentType))

	// 2. Fetch nested CSS file
	req2 := httptest.NewRequest("GET", "/assets/style.css", nil)
	w2 := httptest.NewRecorder()
	s.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, "body { background: #000; }", w2.Body.String())
	assert.Equal(t, "text/css; charset=utf-8", w2.Header().Get(header.ContentType))

	// 3. SPA Fallback for client-side route (/dashboard/settings)
	req3 := httptest.NewRequest("GET", "/dashboard/settings", nil)
	w3 := httptest.NewRecorder()
	s.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusOK, w3.Code)
	assert.Equal(t, "<h1>Welcome to Sein</h1>", w3.Body.String())
}

func TestStatic_DiskFolder_And_ByteRange(t *testing.T) {
	tempDir := t.TempDir()
	testData := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	filePath := filepath.Join(tempDir, "data.txt")
	err := os.WriteFile(filePath, testData, 0644)
	require.NoError(t, err)

	s := sein.New()
	handler := static.New(tempDir, static.Config{
		ByteRange: true,
	})

	s.MountRaw("GET", "/...", handler)

	// Range request: bytes=10-19
	req := httptest.NewRequest("GET", "/data.txt", nil)
	req.Header.Set(header.Range, "bytes=10-19")
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	assert.Equal(t, http.StatusPartialContent, w.Code)
	assert.Equal(t, "bytes 10-19/36", w.Header().Get(header.ContentRange))
	assert.Equal(t, "10", w.Header().Get(header.ContentLength))
	assert.Equal(t, "ABCDEFGHIJ", w.Body.String())
}
