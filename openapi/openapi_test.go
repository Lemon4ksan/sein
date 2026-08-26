// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package openapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/openapi"
)

type UpdateUserDTO struct {
	ID   int    `path:"id"`
	Name string `json:"name"`
}

func TestOpenAPIGenerator(t *testing.T) {
	app := sein.New()

	app.Get("/users/:id", func(ctx context.Context) (string, error) {
		return "user", nil
	})

	app.Post("/users/:id/update", func(ctx context.Context, req UpdateUserDTO) (string, error) {
		return "updated", nil
	})

	doc := openapi.Generate(app, "Test API", "1.0.0")
	require.NotNil(t, doc)
	assert.Equal(t, "3.1.0", doc.OpenAPI)
	assert.Equal(t, "Test API", doc.Info.Title)
	assert.Equal(t, "1.0.0", doc.Info.Version)

	// Check path conversion
	item, ok := doc.Paths["/users/{id}"]
	require.True(t, ok)
	assert.NotNil(t, item["get"])

	jsonBytes, err := doc.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, jsonBytes)
}

func TestEnableDocs(t *testing.T) {
	app := sein.New()

	app.Get("/ping", func(ctx context.Context) (string, error) {
		return "pong", nil
	})

	openapi.EnableDocs(app, "/docs", "Ping API", "2.0.0")

	// Test Scalar HTML
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/docs", nil)
	app.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Contains(t, rec1.Body.String(), "@scalar/api-reference")

	// Test OpenAPI JSON
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil)
	app.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), `"openapi":"3.1.0"`)
}
