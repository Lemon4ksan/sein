// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"github.com/lemon4ksan/sein"
)

func TestServer_MethodNotAllowed_And_AllowHeader(t *testing.T) {
	s := sein.New()

	s.PostAction("/api/v1/users", func(ctx context.Context) (string, error) {
		return "user created", nil
	})
	s.Put("/api/v1/users", func(ctx context.Context, _ struct{}) (string, error) {
		return "user updated", nil
	})

	// Request with unsupported GET method -> 405
	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, "POST, PUT", w.Header().Get(header.Allow))
}

func TestServer_RedirectTrailingSlash(t *testing.T) {
	s := sein.New()

	s.Get("/users", func(ctx context.Context) (string, error) {
		return "users list", nil
	})
	s.Get("/docs/", func(ctx context.Context) (string, error) {
		return "docs root", nil
	})

	// 1. Request /users/ -> should redirect 301 to /users
	req1 := httptest.NewRequest("GET", "/users/", nil)
	w1 := httptest.NewRecorder()
	s.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusMovedPermanently, w1.Code)
	assert.Equal(t, "/users", w1.Header().Get(header.Location))

	// 2. Request /docs -> should redirect 301 to /docs/
	req2 := httptest.NewRequest("GET", "/docs", nil)
	w2 := httptest.NewRecorder()
	s.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusMovedPermanently, w2.Code)
	assert.Equal(t, "/docs/", w2.Header().Get(header.Location))
}

func TestServer_NoRoute_And_NoMethod_Handlers(t *testing.T) {
	s := sein.New()

	s.NoRoute(func(req *sein.Request) (any, error) {
		return sein.StatusWith(http.StatusNotFound, map[string]string{
			"error": "custom_404_not_found",
			"path":  req.Path(),
		}, nil), nil
	})

	s.NoMethod(func(req *sein.Request) (any, error) {
		return sein.StatusWith(http.StatusMethodNotAllowed, map[string]string{
			"error": "custom_405_unsupported_verb",
		}, nil), nil
	})

	s.PostAction("/submit", func(ctx context.Context) (string, error) {
		return "submitted", nil
	})

	// 1. Unmatched Route -> Custom 404
	req1 := httptest.NewRequest("GET", "/unknown/endpoint", nil)
	w1 := httptest.NewRecorder()
	s.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusNotFound, w1.Code)
	assert.True(t, bytes.Contains(w1.Body.Bytes(), []byte("custom_404_not_found")))

	// 2. Unmatched Method -> Custom 405
	req2 := httptest.NewRequest("GET", "/submit", nil)
	w2 := httptest.NewRecorder()
	s.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusMethodNotAllowed, w2.Code)
	assert.True(t, bytes.Contains(w2.Body.Bytes(), []byte("custom_405_unsupported_verb")))
}

func TestServer_RoutesIntrospection(t *testing.T) {
	s := sein.New()

	s.Get("/api/v1/items", func(ctx context.Context) (string, error) { return "items", nil })
	s.Post("/api/v1/items", func(ctx context.Context, _ struct{}) (string, error) { return "created", nil })
	s.Delete("/api/v1/items/:id", func(ctx context.Context) (string, error) { return "deleted", nil })

	routes := s.Routes()
	require.Equal(t, 3, len(routes))

	assert.Equal(t, "GET", routes[0].Method)
	assert.Equal(t, "/api/v1/items", routes[0].Path)

	assert.Equal(t, "POST", routes[1].Method)
	assert.Equal(t, "/api/v1/items", routes[1].Path)

	assert.Equal(t, "DELETE", routes[2].Method)
	assert.Equal(t, "/api/v1/items/:id", routes[2].Path)
}

func TestServer_SaveUploadedFile(t *testing.T) {
	s := sein.New()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "nested", "folder", "uploaded.txt")

	s.PostReq("/upload", func(req *sein.Request, _ struct{}) (any, error) {
		file, err := req.FormFile("document")
		if err != nil {
			return nil, err
		}
		if err := req.SaveUploadedFile(file, targetPath); err != nil {
			return nil, err
		}
		return "saved successfully", nil
	})

	// Build multipart request
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("document", "secret_plan.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte("Top secret server algorithms"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest("POST", "/upload", &body)
	req.Header.Set(header.ContentType, writer.FormDataContentType())
	w := httptest.NewRecorder()

	s.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify file was written to disk
	savedData, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Equal(t, "Top secret server algorithms", string(savedData))
}
