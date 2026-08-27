// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
)

// Group creates a new scoped router group anchored to this server.
func (s *Server) Group(prefix string, mw ...Middleware) *Group {
	return NewGroup(s, prefix, mw...)
}

// Mount attaches a domain Module under the specified prefix with optional group middlewares.
func (s *Server) Mount(prefix string, m Module, mw ...Middleware) *Server {
	g := s.Group(prefix, mw...)
	m.Mount(g)
	return s
}

// Guard creates a protected GuardScope on the server with the specified middlewares applied.
func (s *Server) Guard(mw ...Middleware) *GuardScope {
	return &GuardScope{
		Group: s.Group("", mw...),
	}
}

// MountModule attaches a domain Module directly at root level.
func (s *Server) MountModule(m Module) *Server {
	m.Mount(s.Group(""))
	return s
}

func (s *Server) registerRoute(method, path string, handler RawHandler, mw ...Middleware) {
	Handle(s, method, path, handler, mw...)
}

// MountRaw registers a low-level [RawHandler] on the specified HTTP method and route pattern.
func (s *Server) MountRaw(method, pattern string, handler RawHandler, mw ...Middleware) {
	s.registerRoute(method, pattern, handler, mw...)
}

// Post registers a pure mathematical POST handler on the server: `(context.Context, Req) -> (Res, error)`.
//
// Automatically binds, sanitizes, and validates the request DTO (from JSON, Form, Header, or Query).
//
// # Example
//
//	type CreateUserDTO struct {
//	    Name  string `json:"name,trim,required,min=2"`
//	    Email string `json:"email,lower,email,required"`
//	}
//
//	type UserResponse struct {
//	    ID   int    `json:"id"`
//	    Name string `json:"name"`
//	}
//
//	srv.Post("/users", func(ctx context.Context, req CreateUserDTO) (*UserResponse, error) {
//	    return &UserResponse{ID: 1, Name: req.Name}, nil
//	})
func (s *Server) Post[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePost(s, path, fn, mw...)
}

// PostAction registers a pure parameterless POST handler: `(context.Context) -> (Res, error)`.
func (s *Server) PostAction[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routePostAction(s, path, fn, mw...)
}

// PostWith is an alias for [Server.Post]: `(context.Context, Req) -> (Res, error)`.
func (s *Server) PostWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePost(s, path, fn, mw...)
}

// PostReq registers a POST handler that receives raw [*Request] metadata alongside the parsed DTO.
func (s *Server) PostReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	routePostReq(s, path, fn, mw...)
}

// Get registers a pure parameterless GET handler on the server: `(context.Context) -> (Res, error)`.
//
// # Example
//
//	srv.Get("/health", func(ctx context.Context) (string, error) {
//	    return "OK", nil
//	})
func (s *Server) Get[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routeGet(s, path, fn, mw...)
}

// GetWith registers a pure GET handler with automatic Path/Query/Header DTO binding and validation.
//
// # Example
//
//	type GetUserDTO struct {
//	    ID    int    `path:"id,gt=0"`
//	    Field string `query:"field,default=all"`
//	}
//
//	srv.GetWith("/users/:id", func(ctx context.Context, req GetUserDTO) (*User, error) {
//	    return &User{ID: req.ID}, nil
//	})
func (s *Server) GetWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routeGetWith(s, path, fn, mw...)
}

// GetReq registers a GET handler with access to raw [*Request] metadata: `(*Request) -> (Res, error)`.
func (s *Server) GetReq[Res any](path string, fn func(*Request) (Res, error), mw ...Middleware) {
	routeGetReq(s, path, fn, mw...)
}

// Put registers a pure PUT handler on the server: `(context.Context, Req) -> (Res, error)`.
//
// # Example
//
//	srv.Put("/users/:id", func(ctx context.Context, req UpdateUserDTO) (*User, error) {
//	    return &User{Name: req.Name}, nil
//	})
func (s *Server) Put[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePut(s, path, fn, mw...)
}

// PutAction registers a pure parameterless PUT handler on the server: `(context.Context) -> (Res, error)`.
func (s *Server) PutAction[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routePutAction(s, path, fn, mw...)
}

// PutWith is an alias for [Server.Put]: `(context.Context, Req) -> (Res, error)`.
func (s *Server) PutWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePut(s, path, fn, mw...)
}

// PutReq registers a PUT handler with raw [*Request] metadata: `(*Request, Req) -> (Res, error)`.
func (s *Server) PutReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	routePutReq(s, path, fn, mw...)
}

// Patch registers a pure PATCH handler on the server: `(context.Context, Req) -> (Res, error)`.
func (s *Server) Patch[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routePatch(s, path, fn, mw...)
}

// PatchAction registers a pure parameterless PATCH handler on the server: `(context.Context) -> (Res, error)`.
func (s *Server) PatchAction[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routePatchAction(s, path, fn, mw...)
}

// PatchReq registers a PATCH handler with raw [*Request] metadata: `(*Request, Req) -> (Res, error)`.
func (s *Server) PatchReq[Req, Res any](path string, fn func(*Request, Req) (Res, error), mw ...Middleware) {
	routePatchReq(s, path, fn, mw...)
}

// Delete registers a pure parameterless DELETE handler on the server: `(context.Context) -> (Res, error)`.
func (s *Server) Delete[Res any](path string, fn func(context.Context) (Res, error), mw ...Middleware) {
	routeDelete(s, path, fn, mw...)
}

// DeleteWith registers a pure DELETE handler with Path/Query DTO binding: `(context.Context, Req) -> (Res, error)`.
func (s *Server) DeleteWith[Req, Res any](path string, fn func(context.Context, Req) (Res, error), mw ...Middleware) {
	routeDeleteWith(s, path, fn, mw...)
}

// DeleteReq registers a DELETE handler with raw [*Request] metadata: `(*Request) -> (Res, error)`.
func (s *Server) DeleteReq[Res any](path string, fn func(*Request) (Res, error), mw ...Middleware) {
	routeDeleteReq(s, path, fn, mw...)
}
