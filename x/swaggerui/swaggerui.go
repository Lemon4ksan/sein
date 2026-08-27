// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swaggerui provides zero-dependency interactive Swagger UI / OpenAPI documentation serving.
package swaggerui

import (
	"fmt"
	"strings"

	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// DefaultPath is the standard swagger UI path.
const DefaultPath = "/swagger"

// Config configures Swagger UI.
type Config struct {
	// Path is the URL prefix for the Swagger UI page. Default is "/swagger".
	Path string
	// SpecURL is the URL endpoint serving the OpenAPI JSON/YAML spec. Default is "/swagger/doc.json".
	SpecURL string
	// SpecData contains raw in-memory OpenAPI JSON/YAML specification bytes.
	SpecData []byte
	// Title is the browser tab title. Default is "Swagger UI".
	Title string
}

// Option configures Swagger UI settings.
type Option func(*Config)

// WithPath sets the UI route path.
func WithPath(p string) Option {
	return func(c *Config) {
		c.Path = p
	}
}

// WithSpecURL sets the OpenAPI spec URL.
func WithSpecURL(u string) Option {
	return func(c *Config) {
		c.SpecURL = u
	}
}

// WithSpecData sets in-memory OpenAPI spec data.
func WithSpecData(data []byte) Option {
	return func(c *Config) {
		c.SpecData = data
	}
}

// WithTitle sets the document title.
func WithTitle(title string) Option {
	return func(c *Config) {
		c.Title = title
	}
}

func renderHTML(title, specURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin:0; background: #fafafa; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
    <script>
    window.onload = function() {
        window.ui = SwaggerUIBundle({
            url: "%s",
            dom_id: '#swagger-ui',
            deepLinking: true,
            presets: [
                SwaggerUIBundle.presets.apis,
                SwaggerUIStandalonePreset
            ],
            layout: "StandaloneLayout"
        });
    };
    </script>
</body>
</html>`, title, specURL)
}

// Register registers Swagger UI and spec endpoints directly on the sein server.
func Register(app *sein.Server, opts ...Option) {
	cfg := Config{
		Path:    DefaultPath,
		SpecURL: "/swagger/doc.json",
		Title:   "Swagger UI",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	uiPath := strings.TrimSuffix(cfg.Path, "/")
	htmlContent := renderHTML(cfg.Title, cfg.SpecURL)

	app.Get(uiPath, func(_ *sein.Request) (any, error) {
		return sein.OK[any](htmlContent).
			WithHeader(header.ContentType, header.MIMETextHTMLCharsetUTF8), nil
	})

	app.Get(uiPath+"/", func(_ *sein.Request) (any, error) {
		return sein.OK[any](htmlContent).
			WithHeader(header.ContentType, header.MIMETextHTMLCharsetUTF8), nil
	})

	if len(cfg.SpecData) > 0 {
		app.Get(cfg.SpecURL, func(_ *sein.Request) (any, error) {
			return sein.OK[any](cfg.SpecData).
				WithHeader(header.ContentType, header.MIMEApplicationJSONCharsetUTF8), nil
		})
	}
}
