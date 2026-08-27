// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package expvar provides standard Go runtime expvar diagnostics middleware,
// exposing public counters, gauges, maps, and memory stats under /debug/vars.
package expvar

import (
	"bytes"
	"expvar"
	"net/http"

	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// DefaultPath is the standard expvar diagnostics path.
const DefaultPath = "/debug/vars"

type responseWriter struct {
	headers http.Header
	status  int
	body    bytes.Buffer
}

func (rw *responseWriter) Header() http.Header {
	return rw.headers
}

func (rw *responseWriter) Write(p []byte) (int, error) {
	return rw.body.Write(p)
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
}

// Handler returns a sein.RawHandler serving expvar JSON variables.
func Handler() sein.RawHandler {
	stdHandler := expvar.Handler()

	return func(req *sein.Request) (any, error) {
		rw := &responseWriter{
			headers: make(http.Header),
			status:  http.StatusOK,
		}

		rawReq := req.Raw()
		if rawReq == nil {
			var err error
			rawReq, err = http.NewRequestWithContext(req.Context(), req.Method(), req.Path(), bytes.NewReader(req.RawBody()))
			if err != nil {
				return nil, err
			}
		}

		stdHandler.ServeHTTP(rw, rawReq)

		return sein.OK[any](rw.body.Bytes()).
			WithStatus(rw.status).
			WithHeader(header.ContentType, header.MIMEApplicationJSONCharsetUTF8), nil
	}
}

// New creates an expvar middleware intercepting requests under path (default "/debug/vars").
func New(path ...string) sein.Middleware {
	p := DefaultPath
	if len(path) > 0 && path[0] != "" {
		p = path[0]
	}

	h := Handler()

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			if req.Path() == p {
				return h(req)
			}

			return next(req)
		}
	}
}

// Register registers the expvar endpoint directly onto the sein server.
func Register(app *sein.Server, path ...string) {
	p := DefaultPath
	if len(path) > 0 && path[0] != "" {
		p = path[0]
	}

	app.Get(p, Handler())
}
