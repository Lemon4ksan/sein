// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package crud provides automated RESTful CRUD endpoint mounting for generic repositories (e.g. dawn/orm.Repository[T]).
package crud

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/lemon4ksan/sein"
)

// Repository defines the standard CRUD persistence operations contract.
// It is natively satisfied by *dawn/orm.Repository[T] and any custom ORM/data access repository.
type Repository[T any] interface {
	FindByID(ctx context.Context, id any) (*T, error)
	Create(ctx context.Context, entity *T) error
	Update(ctx context.Context, entity *T) error
	Delete(ctx context.Context, id any) error
}

// FinderAll is an optional repository extension interface for listing entities.
type FinderAll[T any] interface {
	FindAll(ctx context.Context) ([]*T, error)
}

// Router represents any sein router or group capable of registering routes.
type Router interface {
	Get(path string, handler any, mw ...sein.Middleware)
	Post(path string, handler any, mw ...sein.Middleware)
	Put(path string, handler any, mw ...sein.Middleware)
	Patch(path string, handler any, mw ...sein.Middleware)
	Delete(path string, handler any, mw ...sein.Middleware)
}

// Options configures the mounted CRUD resource behavior.
type Options struct {
	IDParam     string
	ReadOnly    bool
	Middlewares []sein.Middleware
}

// Option configures Mount options.
type Option func(*Options)

// WithIDParam customizes the route parameter name for the resource ID (default: "id").
func WithIDParam(param string) Option {
	return func(o *Options) {
		o.IDParam = param
	}
}

// WithReadOnly mounts only read-only endpoints (GET / and GET /:id).
func WithReadOnly() Option {
	return func(o *Options) {
		o.ReadOnly = true
	}
}

// WithMiddleware attaches middleware to all mounted CRUD endpoints.
func WithMiddleware(mw ...sein.Middleware) Option {
	return func(o *Options) {
		o.Middlewares = append(o.Middlewares, mw...)
	}
}

// Mount mounts standard RESTful CRUD endpoints on the provided router or group:
//   - GET    /       -> List all entities
//   - GET    /:id    -> Get entity by ID (404 if not found)
//   - POST   /       -> Create entity (201 Created)
//   - PUT    /:id    -> Update entity (200 OK)
//   - DELETE /:id    -> Delete entity (204 No Content)
//
// # Example
//
//	userRepo := orm.NewRepository[User](db)
//	crud.Mount(app.Group("/users"), userRepo)
func Mount[T any](r Router, repo Repository[T], opts ...Option) {
	MountResource[T](r, repo, opts...)
}

// MountResource mounts standard RESTful CRUD endpoints on the provided router or group.
func MountResource[T any](r Router, repo Repository[T], opts ...Option) {
	cfg := Options{
		IDParam: "id",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	idPath := "/:" + cfg.IDParam

	// 1. GET / -> List entities
	r.Get("/", func(req *sein.Request) (any, error) {
		if finder, ok := repo.(FinderAll[T]); ok {
			items, err := finder.FindAll(req.Context())
			if err != nil {
				return nil, err
			}
			if items == nil {
				return []*T{}, nil
			}
			return items, nil
		}
		return []*T{}, nil
	}, cfg.Middlewares...)

	// 2. GET /:id -> Get entity by ID
	r.Get(idPath, func(req *sein.Request) (any, error) {
		rawID := req.Param(cfg.IDParam).String()
		if rawID == "" {
			return nil, sein.ErrBadRequest("missing resource ID")
		}

		id := parseScalarID(rawID)
		entity, err := repo.FindByID(req.Context(), id)
		if err != nil {
			if isNotFoundError(err) {
				return nil, sein.ErrNotFound("resource not found", err)
			}
			return nil, err
		}
		if entity == nil {
			return nil, sein.ErrNotFound("resource not found")
		}

		return entity, nil
	}, cfg.Middlewares...)

	if cfg.ReadOnly {
		return
	}

	// 3. POST / -> Create entity
	r.Post("/", func(req *sein.Request) (any, error) {
		var entity T
		if err := req.Bind(&entity); err != nil {
			return nil, sein.ErrBadRequest("invalid request payload", err)
		}

		if err := repo.Create(req.Context(), &entity); err != nil {
			return nil, err
		}

		return sein.Created(&entity), nil
	}, cfg.Middlewares...)

	// 4. PUT /:id -> Update entity
	r.Put(idPath, func(req *sein.Request) (any, error) {
		var entity T
		if err := req.Bind(&entity); err != nil {
			return nil, sein.ErrBadRequest("invalid request payload", err)
		}

		if err := repo.Update(req.Context(), &entity); err != nil {
			if isNotFoundError(err) {
				return nil, sein.ErrNotFound("resource not found", err)
			}
			return nil, err
		}

		return &entity, nil
	}, cfg.Middlewares...)

	// 5. DELETE /:id -> Delete entity
	r.Delete(idPath, func(req *sein.Request) (any, error) {
		rawID := req.Param(cfg.IDParam).String()
		if rawID == "" {
			return nil, sein.ErrBadRequest("missing resource ID")
		}

		id := parseScalarID(rawID)
		if err := repo.Delete(req.Context(), id); err != nil {
			if isNotFoundError(err) {
				return nil, sein.ErrNotFound("resource not found", err)
			}
			return nil, err
		}

		return sein.NoContent(), nil
	}, cfg.Middlewares...)
}

func parseScalarID(raw string) any {
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n
	}
	if u, err := strconv.ParseUint(raw, 10, 64); err == nil {
		return u
	}
	return raw
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no rows") || errors.Is(err, http.ErrNoLocation)
}
