// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/generic"
)

// Validatable is an interface for request DTOs that validate their own invariants.
// Any DTO implementing Validatable is automatically validated upon decoding.
type Validatable interface {
	Validate() error
}

type contextSlot struct {
	typ reflect.Type
	val any
}

// Request is a lightweight, read-only view of the incoming HTTP request.
// It has no response-writing methods or mutating side-effects.
// It includes a flat, inline storage array for 0 B/op typed context lookups in L1 CPU cache.
type Request struct {
	ctx       context.Context
	raw       *http.Request
	params    map[string]string
	scope     *borrow.Scope
	slots     [8]contextSlot
	slotCount int
	overflow  map[reflect.Type]any
	bodyMu    sync.Mutex
	bodyBuf   []byte
}

// NewRequest creates a Request wrapping a standard http.Request.
func NewRequest(r *http.Request, params map[string]string) *Request {
	return &Request{
		ctx:    r.Context(),
		raw:    r,
		params: params,
		scope:  borrow.NewScope(),
	}
}

// Scope returns the request-scoped lexical memory arena.
func (r *Request) Scope() *borrow.Scope {
	if r.scope == nil {
		r.scope = borrow.NewScope()
	}
	return r.scope
}

// Release recycles the request's internal memory scope.
func (r *Request) Release() {
	if r.scope != nil {
		r.scope.Release()
		r.scope = nil
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

// BindJSON decodes the JSON request body into dest and executes automatic validation if dest implements Validatable.
func (r *Request) BindJSON(dest any) error {
	body := r.Body()
	if len(body) == 0 {
		return BadRequest("EMPTY_REQUEST_BODY", "Request body cannot be empty")
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return BadRequest("INVALID_JSON_PAYLOAD", "Invalid JSON payload structure").WithCause(err)
	}

	if v, ok := dest.(Validatable); ok {
		if err := v.Validate(); err != nil {
			if domainErr, ok := errors.AsType[DomainError](err); ok {
				return domainErr
			}
			return BadRequest("VALIDATION_FAILED", err.Error())
		}
	}
	return nil
}

// Set stores a typed value in the flat inline context storage (0 B/op, L1 cache scan).
func Set[T any](r *Request, val T) {
	typ := reflect.TypeFor[T]()

	for i := range r.slotCount {
		if r.slots[i].typ == typ {
			r.slots[i].val = val
			return
		}
	}

	if r.slotCount < len(r.slots) {
		r.slots[r.slotCount] = contextSlot{typ: typ, val: val}
		r.slotCount++
		return
	}

	if r.overflow == nil {
		r.overflow = make(map[reflect.Type]any)
	}
	r.overflow[typ] = val
}

// Get retrieves a typed value from the flat inline context storage in L1 CPU cache.
func Get[T any](r *Request) (T, bool) {
	typ := reflect.TypeFor[T]()

	for i := range r.slotCount {
		if r.slots[i].typ == typ {
			typed, ok := r.slots[i].val.(T)
			return typed, ok
		}
	}

	if r.overflow != nil {
		if v, ok := r.overflow[typ]; ok {
			typed, ok := v.(T)
			return typed, ok
		}
	}

	return generic.Zero[T](), false
}

// MustGet retrieves a typed value or panics if not present.
func MustGet[T any](r *Request) T {
	val, ok := Get[T](r)
	if !ok {
		panic("sein: requested type not found in request storage: " + reflect.TypeFor[T]().String())
	}
	return val
}
