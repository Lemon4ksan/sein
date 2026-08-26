// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"slices"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/lemon4ksan/foundation/net/http/header"
)

// Option configures a sein Server instance.
type Option func(s *Server)

// ErrorMapper translates arbitrary errors into typed DomainErrors.
type ErrorMapper func(err error) (DomainError, bool)

// Server is the unified high-performance protocol server.
type Server struct {
	addr         string
	router       *Router
	middlewares  []Middleware
	errorMappers []ErrorMapper
	httpServer   *http.Server
	mu           sync.Mutex
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

// MapError registers a mapping from a sentinel error target to a DomainError.
func (s *Server) MapError(target error, domainErr DomainError) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorMappers = append(s.errorMappers, func(err error) (DomainError, bool) {
		if errors.Is(err, target) {
			return domainErr, true
		}
		return nil, false
	})
	return s
}

// MapErrorFunc registers a custom error mapping predicate.
func (s *Server) MapErrorFunc(fn ErrorMapper) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorMappers = append(s.errorMappers, fn)
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

// Post registers a pure POST handler on the server: (ctx, Req) -> (Res, error)
func (s *Server) Post[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePost(s, path, fn, mw...)
}

// PostAction registers a pure parameterless POST handler on the server: (ctx) -> (Res, error)
func (s *Server) PostAction[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routePostAction(s, path, fn, mw...)
}

// PostWith is an alias for Post on the server: (ctx, Req) -> (Res, error)
func (s *Server) PostWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePost(s, path, fn, mw...)
}

// PostReq registers a POST handler with Request metadata on the server: (req, Req) -> (Res, error)
func (s *Server) PostReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	routePostReq(s, path, fn, mw...)
}

// Get registers a pure GET handler on the server: (ctx) -> (Res, error)
func (s *Server) Get[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routeGet(s, path, fn, mw...)
}

// GetWith registers a pure GET handler with Path/Query/Header DTO on the server: (ctx, Req) -> (Res, error)
func (s *Server) GetWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routeGetWith(s, path, fn, mw...)
}

// GetReq registers a GET handler with Request metadata on the server: (req) -> (Res, error)
func (s *Server) GetReq[Res any](path string, fn func(*Request) (Res, error), mw ...Middleware) {
	routeGetReq(s, path, fn, mw...)
}

// Put registers a pure PUT handler on the server: (ctx, Req) -> (Res, error)
func (s *Server) Put[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePut(s, path, fn, mw...)
}

// PutWith is an alias for Put on the server: (ctx, Req) -> (Res, error)
func (s *Server) PutWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePut(s, path, fn, mw...)
}

// PutReq registers a PUT handler with Request metadata on the server: (req, Req) -> (Res, error)
func (s *Server) PutReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	routePutReq(s, path, fn, mw...)
}

// Patch registers a pure PATCH handler on the server: (ctx, Req) -> (Res, error)
func (s *Server) Patch[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePatch(s, path, fn, mw...)
}

// PatchWith is an alias for Patch on the server: (ctx, Req) -> (Res, error)
func (s *Server) PatchWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePatch(s, path, fn, mw...)
}

// PatchReq registers a PATCH handler with Request metadata on the server: (req, Req) -> (Res, error)
func (s *Server) PatchReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	routePatchReq(s, path, fn, mw...)
}

// Delete registers a pure DELETE handler on the server: (ctx) -> (Res, error)
func (s *Server) Delete[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routeDelete(s, path, fn, mw...)
}

// DeleteWith registers a pure DELETE handler with Path/Query DTO on the server: (ctx, Req) -> (Res, error)
func (s *Server) DeleteWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routeDeleteWith(s, path, fn, mw...)
}

// DeleteReq registers a DELETE handler with Request metadata on the server: (req) -> (Res, error)
func (s *Server) DeleteReq[Res any](path string, fn func(*Request) (Res, error), mw ...Middleware) {
	routeDeleteReq(s, path, fn, mw...)
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
	for _, v := range slices.Backward(s.middlewares) {
		finalHandler = v(finalHandler)
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
	for _, mapper := range s.errorMappers {
		if mapped, ok := mapper(err); ok {
			err = mapped
			break
		}
	}

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

	w.Header().Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)
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
