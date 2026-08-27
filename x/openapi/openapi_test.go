// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package openapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/x/openapi"
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

type ComplexSearchDTO struct {
	Query     string            `query:"q,required"`
	Page      int               `query:"page"`
	Auth      string            `header:"Authorization,required"`
	Session   string            `cookie:"session_id"`
	Active    bool              `json:"active"`
	Score     float64           `json:"score"`
	Tags      []string          `json:"tags"`
	Metadata  map[string]string `json:"metadata"`
	RawBytes  []byte            `json:"raw_bytes"`
	CreatedAt time.Time         `json:"created_at"`
}

type SearchResponse struct {
	Total int      `json:"total"`
	Items []string `json:"items"`
}

func TestOpenAPI_Export_And_ComplexTypes(t *testing.T) {
	app := sein.New()

	app.Post("/search/*wildcard", func(ctx context.Context, dto ComplexSearchDTO) (SearchResponse, error) {
		return SearchResponse{Total: 1, Items: []string{"found"}}, nil
	})

	doc := openapi.Generate(app, "Complex API", "3.1.0")
	require.NotNil(t, doc)

	// Test Export to disk
	tmpFile := t.TempDir() + "/spec.json"
	err := doc.Export(tmpFile)
	require.NoError(t, err)

	data, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"openapi":"3.1.0"`)
	assert.Contains(t, string(data), "Complex API")
}
