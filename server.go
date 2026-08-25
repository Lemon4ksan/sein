// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
)

// Option configures a sein Server instance.
type Option func(s *Server)

// Server is the unified high-performance protocol server.
type Server struct {
	addr        string
	router      *Router
	middlewares []Middleware
	httpServer  *http.Server
	mu          sync.Mutex
}

// WithAddr sets the listening address.
func WithAddr(addr string) Option {
	return func(s *Server) {
		s.addr = addr
	}
}

// New creates a new sein Server instance.
func New(opts ...Option) *Server {
	s := &Server{
		addr:   ":8080",
		router: NewRouter(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Use appends global middleware to the server pipeline.
func (s *Server) Use(mw ...Middleware) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.middlewares = append(s.middlewares, mw...)
}

// Group creates a new scoped router group anchored to this server.
func (s *Server) Group(prefix string, mw ...Middleware) *Group {
	return NewGroup(s, prefix, mw...)
}

func (s *Server) registerRoute(method, path string, handler RawHandler, mw ...Middleware) {
	Handle(s, method, path, handler, mw...)
}

// ServeHTTP satisfies the standard http.Handler interface, enabling seamless interoperability with Go stdlib.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler, params, found := s.router.Match(r.Method, r.URL.Path)
	if !found {
		s.writeError(w, ErrNotFound("route not found"))
		return
	}

	req := NewRequest(r, params)
	defer req.Release()

	// Wrap in global middlewares
	finalHandler := handler
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		finalHandler = s.middlewares[i](finalHandler)
	}

	// Execute handler
	result, err := finalHandler(req)
	if err != nil {
		s.writeError(w, err)
		return
	}

	if responder, ok := result.(Responder); ok {
		if err := responder.WriteResponse(w); err != nil {
			s.writeError(w, ErrInternal("failed to write response", err))
		}
		return
	}

	// Fallback to OK JSON
	_ = OK(result).WriteResponse(w)
}

type errorResponse struct {
	Status  int            `json:"status"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (s *Server) writeError(w http.ResponseWriter, err error) {
	var resp errorResponse

	var definedErr DefinedError
	var domainErr DomainError
	var httpErr HTTPError

	switch {
	case errors.As(err, &definedErr):
		resp = errorResponse{
			Status:  definedErr.HTTPStatus(),
			Code:    definedErr.ErrorCode(),
			Message: definedErr.Message(),
			Details: definedErr.Details(),
		}
	case errors.As(err, &domainErr):
		resp = errorResponse{
			Status:  domainErr.HTTPStatus(),
			Code:    domainErr.ErrorCode(),
			Message: domainErr.Error(),
		}
	case errors.As(err, &httpErr):
		resp = errorResponse{
			Status:  httpErr.HTTPStatus(),
			Code:    httpErr.ErrorCode(),
			Message: httpErr.Message,
			Details: httpErr.Details,
		}
	default:
		resp = errorResponse{
			Status:  http.StatusInternalServerError,
			Code:    "INTERNAL_SERVER_ERROR",
			Message: err.Error(),
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(resp.Status)
	_ = json.NewEncoder(w).Encode(resp)
}

// ListenAndServe starts the HTTP server listening on the configured address.
func (s *Server) ListenAndServe() error {
	s.mu.Lock()
	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: s,
	}
	s.mu.Unlock()

	return s.httpServer.ListenAndServe()
}

// Listen starts listening on the specified address.
func (s *Server) Listen(addr string) error {
	s.addr = addr
	return s.ListenAndServe()
}

// Serve starts serving connections from the provided net.Listener.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	s.httpServer = &http.Server{
		Addr:    s.addr,
		Handler: s,
	}
	s.mu.Unlock()

	return s.httpServer.Serve(ln)
}

// Close gracefully closes the server.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}
