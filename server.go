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

// ServeHTTP satisfies the standard http.Handler interface, enabling seamless interoperability with Go stdlib.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler, params, found := s.router.Match(r.Method, r.URL.Path)
	if !found {
		s.writeError(w, ErrNotFound("route not found"))
		return
	}

	req := NewRequest(r, params)

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

func (s *Server) writeError(w http.ResponseWriter, err error) {
	var httpErr HTTPError
	if !errors.As(err, &httpErr) {
		httpErr = ErrInternal(err.Error())
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpErr.StatusCode())
	_ = json.NewEncoder(w).Encode(httpErr)
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
