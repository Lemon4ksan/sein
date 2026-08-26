// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pprof provides Go runtime profiling endpoints under /debug/pprof/
// for live production performance inspection and memory leak analysis.
package pprof

import (
	"bytes"
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/lemon4ksan/sein"
)

// DefaultPrefix is the canonical pprof routing prefix.
const DefaultPrefix = "/debug/pprof"

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

func adapt(handler http.HandlerFunc) sein.RawHandler {
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

		handler(rw, rawReq)

		resp := sein.OK[any](rw.body.Bytes()).WithStatus(rw.status)
		for k, vv := range rw.headers {
			for _, v := range vv {
				resp = resp.WithHeader(k, v)
			}
		}

		return resp, nil
	}
}

// New creates a pprof middleware routing requests under prefix (default "/debug/pprof")
// directly to the corresponding Go runtime profilers.
func New(prefix ...string) sein.Middleware {
	p := DefaultPrefix
	if len(prefix) > 0 && prefix[0] != "" {
		p = strings.TrimSuffix(prefix[0], "/")
	}

	routes := map[string]http.HandlerFunc{
		p:             pprof.Index,
		p + "/":       pprof.Index,
		p + "/cmdline": pprof.Cmdline,
		p + "/profile": pprof.Profile,
		p + "/symbol":  pprof.Symbol,
		p + "/trace":   pprof.Trace,
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			path := req.Path()

			if handler, ok := routes[path]; ok {
				return adapt(handler)(req)
			}

			if strings.HasPrefix(path, p+"/") {
				return adapt(pprof.Index)(req)
			}

			return next(req)
		}
	}
}

// Register registers all pprof diagnostic endpoints directly onto the sein server.
func Register(app *sein.Server, prefix ...string) {
	p := DefaultPrefix
	if len(prefix) > 0 && prefix[0] != "" {
		p = strings.TrimSuffix(prefix[0], "/")
	}

	app.GetReq(p, adapt(pprof.Index))
	app.GetReq(p+"/", adapt(pprof.Index))
	app.GetReq(p+"/cmdline", adapt(pprof.Cmdline))
	app.GetReq(p+"/profile", adapt(pprof.Profile))
	app.GetReq(p+"/symbol", adapt(pprof.Symbol))
	app.GetReq(p+"/trace", adapt(pprof.Trace))
	app.GetReq(p+"/goroutine", adapt(pprof.Index))
	app.GetReq(p+"/heap", adapt(pprof.Index))
	app.GetReq(p+"/threadcreate", adapt(pprof.Index))
	app.GetReq(p+"/block", adapt(pprof.Index))
	app.GetReq(p+"/mutex", adapt(pprof.Index))
	app.GetReq(p+"/allocs", adapt(pprof.Index))
}
