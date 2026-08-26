// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package openapi provides automated OpenAPI 3.1.0 document generation and interactive Scalar / Swagger UI rendering for sein.
package openapi

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/sein"
)

// Document represents an OpenAPI 3.1.0 root document conforming to https://spec.openapis.org/oas/v3.1.0.
type Document struct {
	OpenAPI string                          `json:"openapi"`
	Info    Info                            `json:"info"`
	Paths   map[string]map[string]Operation `json:"paths"`
}

// Info provides metadata about the API.
type Info struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

// Operation describes a single API operation on a path.
type Operation struct {
	Summary     string              `json:"summary,omitempty"`
	Description string              `json:"description,omitempty"`
	Parameters  []Parameter         `json:"parameters,omitempty"`
	Responses   map[string]Response `json:"responses"`
}

// Parameter describes a single operation parameter (path or query).
type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"` // "path" or "query"
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
	Schema      Schema `json:"schema"`
}

// Response describes an expected HTTP response.
type Response struct {
	Description string `json:"description"`
}

// Schema describes a data type.
type Schema struct {
	Type string `json:"type"`
}

// Generate builds an OpenAPI 3.1.0 Document from all routes registered on the sein server.
func Generate(s *sein.Server, title, version string) *Document {
	doc := &Document{
		OpenAPI: "3.1.0",
		Info: Info{
			Title:   title,
			Version: version,
		},
		Paths: make(map[string]map[string]Operation),
	}

	routes := s.Routes()
	for _, r := range routes {
		openApiPath, params := convertRouteToOpenAPI(r.Path)

		if doc.Paths[openApiPath] == nil {
			doc.Paths[openApiPath] = make(map[string]Operation)
		}

		methodKey := strings.ToLower(r.Method)
		doc.Paths[openApiPath][methodKey] = Operation{
			Summary:    fmt.Sprintf("%s %s", r.Method, r.Path),
			Parameters: params,
			Responses: map[string]Response{
				"200": {Description: "Successful operation"},
				"400": {Description: "Bad request"},
				"401": {Description: "Unauthorized"},
				"500": {Description: "Internal server error"},
			},
		}
	}

	return doc
}

// ToJSON serializes the document into pretty-printed JSON bytes.
func (d *Document) ToJSON() ([]byte, error) {
	return json.Marshal(d)
}

// Export writes the OpenAPI specification directly to a local JSON file.
func (d *Document) Export(filepath string) error {
	data, err := d.ToJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath, data, 0o600)
}

func convertRouteToOpenAPI(routePath string) (string, []Parameter) {
	var params []Parameter
	segments := strings.Split(routePath, "/")

	for i, seg := range segments {
		if strings.HasPrefix(seg, ":") {
			paramName := strings.TrimPrefix(seg, ":")
			segments[i] = "{" + paramName + "}"
			params = append(params, Parameter{
				Name:     paramName,
				In:       "path",
				Required: true,
				Schema:   Schema{Type: "string"},
			})
		} else if strings.HasPrefix(seg, "*") {
			paramName := strings.TrimPrefix(seg, "*")
			segments[i] = "{" + paramName + "}"
			params = append(params, Parameter{
				Name:     paramName,
				In:       "path",
				Required: true,
				Schema:   Schema{Type: "string"},
			})
		}
	}

	return strings.Join(segments, "/"), params
}

// EnableDocs registers automatic OpenAPI spec serving and an interactive Scalar documentation UI on the server.
func EnableDocs(s *sein.Server, mountPath, title, version string) {
	base := strings.TrimSuffix(mountPath, "/")
	specURL := base + "/openapi.json"

	doc := Generate(s, title, version)
	specJSON, _ := doc.ToJSON()

	// Serve raw OpenAPI 3.1 JSON
	s.Get(specURL, func(_ context.Context) (any, error) {
		return sein.OK[any](specJSON).
			WithHeader(header.ContentType, header.MIMEApplicationJSONCharsetUTF8), nil
	})

	// Serve Scalar API Reference UI
	scalarHTML := renderScalarHTML(title, specURL)
	s.Get(base, func(_ context.Context) (any, error) {
		return sein.OK[any](scalarHTML).
			WithHeader(header.ContentType, header.MIMETextHTMLCharsetUTF8), nil
	})
	s.Get(base+"/", func(_ context.Context) (any, error) {
		return sein.OK[any](scalarHTML).
			WithHeader(header.ContentType, header.MIMETextHTMLCharsetUTF8), nil
	})
}

func renderScalarHTML(title, specURL string) string {
	return fmt.Sprintf(`<!doctype html>
<html>
  <head>
    <title>%s</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
  </head>
  <body>
    <script id="api-reference" data-url="%s"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>`, title, specURL)
}
