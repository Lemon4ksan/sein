// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpc

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/internal/fast/h1engine"
)

type grpcResponder struct {
	srv *Server
	req *sein.Request
}

func (g grpcResponder) WriteToH1(res *h1engine.Response) error {
	rec := httptest.NewRecorder()
	var httpReq *http.Request
	if g.req.Raw() != nil {
		httpReq = g.req.Raw()
	} else {
		p := g.req.Path()
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		httpReq = httptest.NewRequestWithContext(g.req.Context(), g.req.Method(), p, bytes.NewReader(g.req.Body()))
		httpReq.Header.Set("Content-Type", g.req.Header("Content-Type"))
	}

	g.srv.ServeHTTP(rec, httpReq)

	res.StatusCode = rec.Code
	res.Headers.AddFromHTTP(rec.Header())
	res.Body = append(res.Body[:0], rec.Body.Bytes()...)

	return nil
}

// Mount mounts the gRPC server onto a Sein server instance, allowing both
// standard HTTP/REST endpoints and high-performance gRPC services to coexist on the exact same port.
func (s *Server) Mount(router sein.RouteBuilder) {
	s.mu.RLock()
	services := make([]string, 0, len(s.services))
	for name := range s.services {
		services = append(services, name)
	}
	s.mu.RUnlock()

	for _, svcName := range services {
		prefix := "/" + svcName + "/:method"

		sein.Handle(router, http.MethodPost, prefix, func(req *sein.Request) (any, error) {
			return grpcResponder{srv: s, req: req}, nil
		})
	}
}

// Middleware returns a Sein middleware that automatically intercepts and handles gRPC requests.
func (s *Server) Middleware() sein.Middleware {
	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			ct := req.Header("Content-Type")
			if strings.HasPrefix(ct, "application/grpc") {
				return grpcResponder{srv: s, req: req}, nil
			}

			return next(req)
		}
	}
}
