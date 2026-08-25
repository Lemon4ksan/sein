// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
)

// Request is a lightweight, read-only view of the incoming HTTP request.
// It has no response-writing methods or mutating side-effects.
type Request struct {
	ctx     context.Context
	raw     *http.Request
	params  map[string]string
	storage map[reflect.Type]any
	bodyMu  sync.Mutex
	bodyBuf []byte
}

// NewRequest creates a Request wrapping a standard http.Request.
func NewRequest(r *http.Request, params map[string]string) *Request {
	return &Request{
		ctx:     r.Context(),
		raw:     r,
		params:  params,
		storage: make(map[reflect.Type]any),
	}
}

// Context returns the request-scoped context.
func (r *Request) Context() context.Context {
	if r.ctx == nil {
		return context.Background()
	}
	return r.ctx
}

// WithContext sets a new context on the request.
func (r *Request) WithContext(ctx context.Context) *Request {
	r.ctx = ctx
	return r
}

// Raw returns the underlying *http.Request for advanced compatibility.
func (r *Request) Raw() *http.Request {
	return r.raw
}

// Method returns the HTTP method (e.g. GET, POST).
func (r *Request) Method() string {
	return r.raw.Method
}

// Path returns the requested URL path.
func (r *Request) Path() string {
	return r.raw.URL.Path
}

// Param retrieves a URL path parameter by name (e.g. "id" for "/users/:id").
func (r *Request) Param(name string) ParamValue {
	if r.params == nil {
		return ""
	}
	return ParamValue(r.params[name])
}

// Query retrieves a query parameter by key.
func (r *Request) Query(key string) ParamValue {
	return ParamValue(r.raw.URL.Query().Get(key))
}

// Header retrieves an HTTP request header by key.
func (r *Request) Header(key string) string {
	return r.raw.Header.Get(key)
}

// BearerToken extracts the token from the "Authorization: Bearer <token>" header.
func (r *Request) BearerToken() (string, bool) {
	auth := r.Header("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		return strings.TrimSpace(token), true
	}
	return "", false
}

// Cookie retrieves a cookie value by name.
func (r *Request) Cookie(name string) (string, error) {
	c, err := r.raw.Cookie(name)
	if err != nil {
		return "", err
	}
	return c.Value, nil
}

// Body reads and caches the full request body.
func (r *Request) Body() []byte {
	r.bodyMu.Lock()
	defer r.bodyMu.Unlock()

	if r.bodyBuf != nil {
		return r.bodyBuf
	}

	if r.raw.Body == nil {
		return nil
	}

	data, err := io.ReadAll(r.raw.Body)
	if err != nil {
		return nil
	}
	r.bodyBuf = data
	return data
}

// BindJSON decodes the JSON request body into dest.
func (r *Request) BindJSON(dest any) error {
	body := r.Body()
	if len(body) == 0 {
		return ErrBadRequest("empty request body")
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return ErrBadRequest("invalid JSON payload", err)
	}
	return nil
}

// Set stores a typed value in the request storage indexed by its Go type.
func Set[T any](r *Request, val T) {
	if r.storage == nil {
		r.storage = make(map[reflect.Type]any)
	}
	r.storage[reflect.TypeFor[T]()] = val
}

// Get retrieves a typed value from the request storage.
func Get[T any](r *Request) (T, bool) {
	if r.storage == nil {
		var zero T
		return zero, false
	}
	v, ok := r.storage[reflect.TypeFor[T]()]
	if !ok {
		var zero T
		return zero, false
	}
	typed, ok := v.(T)
	return typed, ok
}

// MustGet retrieves a typed value or panics if not present.
func MustGet[T any](r *Request) T {
	val, ok := Get[T](r)
	if !ok {
		panic("sein: requested type not found in request storage: " + reflect.TypeFor[T]().String())
	}
	return val
}
