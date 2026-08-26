// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/sein/internal/compress"
	"github.com/lemon4ksan/sein/internal/h1"
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
	ctx           context.Context
	method        string
	path          string
	query         string
	proto         string
	host          string
	remoteAddr    string
	bodyBuf       []byte
	h1Headers     *h1.Headers
	h1Req         *h1.Request
	raw           *http.Request
	multipartForm *multipart.Form
	params        map[string]string
	scope         *borrow.Scope
	slots         [8]contextSlot
	slotCount     int
	overflow      map[reflect.Type]any
	bodyMu        sync.Mutex
}

// NewRequest creates a Request wrapping a standard http.Request.
func NewRequest(r *http.Request, params map[string]string) *Request {
	req := &Request{
		ctx:        r.Context(),
		raw:        r,
		method:     r.Method,
		params:     params,
		scope:      borrow.NewScope(),
		remoteAddr: r.RemoteAddr,
		proto:      r.Proto,
		host:       r.Host,
	}
	if r.URL != nil {
		req.path = r.URL.Path
		req.query = r.URL.RawQuery
	}
	return req
}

// NewH1Request creates a Request wrapping a native zero-net/http h1.Request.
func NewH1Request(h1Req *h1.Request, params map[string]string) *Request {
	return &Request{
		ctx:        context.Background(),
		method:     h1Req.Method,
		path:       h1Req.Path,
		query:      h1Req.Query,
		proto:      h1Req.Proto,
		host:       h1Req.Host,
		remoteAddr: h1Req.RemoteAddr,
		bodyBuf:    h1Req.Body,
		h1Headers:  &h1Req.Headers,
		h1Req:      h1Req,
		params:     params,
		scope:      borrow.NewScope(),
	}
}

// NewH2Request creates a Request wrapping a native H2 stream request.
func NewH2Request(method, path, authority, remoteAddr string, rawHeaders http.Header, body []byte, params map[string]string) *Request {
	req := &Request{
		ctx:        context.Background(),
		method:     method,
		path:       path,
		proto:      "HTTP/2.0",
		host:       authority,
		remoteAddr: remoteAddr,
		bodyBuf:    body,
		params:     params,
		scope:      borrow.NewScope(),
	}
	if rawHeaders != nil {
		h := h1.NewHeadersWithCapacity(len(rawHeaders))
		for k, vv := range rawHeaders {
			for _, v := range vv {
				h.Set(k, v)
			}
		}
		req.h1Headers = &h
	}
	return req
}

// Hijack takes over the raw underlying TCP connection from the server.
// Once hijacked, the server will not write any HTTP response and will not close the connection.
func (r *Request) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if r.h1Req != nil {
		return r.h1Req.Hijack()
	}
	if r.raw != nil {
		if hj, ok := any(r.raw).(http.Hijacker); ok {
			return hj.Hijack()
		}
	}
	return nil, nil, errors.New("sein: hijacking not supported on this connection")
}

// Scope returns the request-scoped lexical memory arena.
func (r *Request) Scope() *borrow.Scope {
	if r.scope == nil {
		r.scope = borrow.NewScope()
	}
	return r.scope
}

// Release recycles the request's internal memory scope and cleans up multipart files.
func (r *Request) Release() {
	if r.scope != nil {
		r.scope.Release()
		r.scope = nil
	}
	if r.multipartForm != nil {
		_ = r.multipartForm.RemoveAll()
		r.multipartForm = nil
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

// Raw returns the underlying *http.Request for advanced compatibility if available.
func (r *Request) Raw() *http.Request {
	return r.raw
}

// Method returns the HTTP method (e.g. GET, POST).
func (r *Request) Method() string {
	if r.method != "" {
		return r.method
	}
	if r.raw != nil {
		return r.raw.Method
	}
	return ""
}

// Path returns the requested URL path.
func (r *Request) Path() string {
	if r.path != "" {
		return r.path
	}
	if r.raw != nil && r.raw.URL != nil {
		return r.raw.URL.Path
	}
	return ""
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
	if r.query != "" {
		for _, pair := range strings.Split(r.query, "&") {
			if k, v, found := strings.Cut(pair, "="); found {
				if k == key {
					unescaped, err := url.QueryUnescape(v)
					if err == nil {
						return ParamValue(unescaped)
					}
					return ParamValue(v)
				}
			} else if pair == key {
				return ""
			}
		}
		return ""
	}
	if r.raw != nil && r.raw.URL != nil {
		return ParamValue(r.raw.URL.Query().Get(key))
	}
	return ""
}

// Header retrieves an HTTP request header by key.
func (r *Request) Header(key string) string {
	if r.h1Headers != nil {
		return r.h1Headers.Get(key)
	}
	if r.raw != nil {
		return r.raw.Header.Get(key)
	}
	return ""
}

// BearerToken extracts the token from the "Authorization: Bearer <token>" header.
func (r *Request) BearerToken() (string, bool) {
	auth := r.Header(header.Authorization)
	prefix := header.ValueBearer + " "
	if strings.HasPrefix(auth, prefix) {
		token := strings.TrimPrefix(auth, prefix)
		return strings.TrimSpace(token), true
	}
	return "", false
}

// ClientIP returns the real client IP address, checking CF-Connecting-IP, X-Real-IP, X-Forwarded-For and RemoteAddr.
func (r *Request) ClientIP() string {
	if cfIP := r.Header(header.CFConnectingIP); cfIP != "" {
		return strings.TrimSpace(cfIP)
	}
	if realIP := r.Header(header.XRealIP); realIP != "" {
		return strings.TrimSpace(realIP)
	}
	if fwd := r.Header(header.XForwardedFor); fwd != "" {
		if ip, _, found := strings.Cut(fwd, ","); found {
			return strings.TrimSpace(ip)
		}
		return strings.TrimSpace(fwd)
	}
	if r.remoteAddr != "" {
		host, _, err := net.SplitHostPort(r.remoteAddr)
		if err == nil {
			return host
		}
		return r.remoteAddr
	}
	if r.raw != nil {
		host, _, err := net.SplitHostPort(r.raw.RemoteAddr)
		if err == nil {
			return host
		}
		return r.raw.RemoteAddr
	}
	return ""
}

// Protocol returns the network protocol (e.g. "HTTP/1.1", "HTTP/2.0", "HTTP/3.0").
func (r *Request) Protocol() string {
	if r.proto != "" {
		return r.proto
	}
	if r.raw != nil {
		return r.raw.Proto
	}
	return ""
}

// Scheme returns the request scheme ("https" or "http").
func (r *Request) Scheme() string {
	if proto := r.Header(header.XForwardedProto); proto != "" {
		return proto
	}
	if r.raw != nil {
		if r.raw.TLS != nil {
			return "https"
		}
		if r.raw.URL != nil && r.raw.URL.Scheme != "" {
			return r.raw.URL.Scheme
		}
	}
	return "http"
}

// Host returns the request target host (Host header or URL host).
func (r *Request) Host() string {
	if r.host != "" {
		return r.host
	}
	if r.raw != nil {
		return r.raw.Host
	}
	return ""
}

// IfNoneMatch reports whether the client's If-None-Match header matches etag (RFC 7232 §3.2).
func (r *Request) IfNoneMatch(etag string) bool {
	headerVal := r.Header(header.IfNoneMatch)
	if headerVal == "" {
		return false
	}
	if headerVal == "*" {
		return true
	}
	etag = strings.Trim(etag, "\"")
	for _, part := range strings.Split(headerVal, ",") {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "W/")
		part = strings.Trim(part, "\"")
		if part == etag {
			return true
		}
	}
	return false
}

// IfModifiedSince reports whether the resource has not been modified since the client's header timestamp (RFC 7232 §3.3).
func (r *Request) IfModifiedSince(lastModified time.Time) bool {
	headerVal := r.Header(header.IfModifiedSince)
	if headerVal == "" {
		return false
	}
	t, err := http.ParseTime(headerVal)
	if err != nil {
		return false
	}
	return !lastModified.Truncate(time.Second).After(t)
}

func (r *Request) parseMultipartForm(maxMemory int64) error {
	if r.multipartForm != nil {
		return nil
	}

	ct := r.Header(header.ContentType)
	if !strings.HasPrefix(ct, "multipart/form-data") {
		return errors.New("sein: request Content-Type is not multipart/form-data")
	}

	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return err
	}

	boundary, ok := params["boundary"]
	if !ok {
		return errors.New("sein: no multipart boundary in Content-Type")
	}

	mr := multipart.NewReader(bytes.NewReader(r.Body()), boundary)
	f, err := mr.ReadForm(maxMemory)
	if err != nil {
		return err
	}

	r.multipartForm = f
	return nil
}

// FormValue retrieves a value from POST/PUT form-encoded or multipart data.
func (r *Request) FormValue(key string) string {
	if r.raw != nil {
		if v := r.raw.FormValue(key); v != "" {
			return v
		}
	}

	if err := r.parseMultipartForm(32 << 20); err == nil && r.multipartForm != nil {
		if vs := r.multipartForm.Value[key]; len(vs) > 0 {
			return vs[0]
		}
	}

	ct := r.Header(header.ContentType)
	if strings.HasPrefix(ct, header.MIMEApplicationForm) {
		body := string(r.Body())
		for _, pair := range strings.Split(body, "&") {
			if k, v, found := strings.Cut(pair, "="); found && k == key {
				unescaped, err := url.QueryUnescape(v)
				if err == nil {
					return unescaped
				}
				return v
			}
		}
	}

	return ""
}

// Cookie retrieves a cookie value by name.
func (r *Request) Cookie(name string) (string, error) {
	cookieHeader := r.Header(header.Cookie)
	if cookieHeader == "" {
		return "", http.ErrNoCookie
	}
	for _, part := range strings.Split(cookieHeader, ";") {
		part = strings.TrimSpace(part)
		if k, v, found := strings.Cut(part, "="); found {
			if k == name {
				return v, nil
			}
		}
	}
	return "", http.ErrNoCookie
}

// FormFile retrieves an uploaded file from multipart form data.
func (r *Request) FormFile(key string) (*File, error) {
	if r.raw != nil {
		_, fh, err := r.raw.FormFile(key)
		if err == nil && fh != nil {
			return NewFile(fh), nil
		}
	}

	if err := r.parseMultipartForm(32 << 20); err != nil {
		return nil, err
	}

	if r.multipartForm != nil && r.multipartForm.File != nil {
		fhs := r.multipartForm.File[key]
		if len(fhs) > 0 {
			return NewFile(fhs[0]), nil
		}
	}

	return nil, http.ErrMissingFile
}

// FormFiles retrieves all uploaded files under key from multipart form data.
func (r *Request) FormFiles(key string) ([]*File, error) {
	if r.raw != nil && r.raw.MultipartForm != nil {
		fhs := r.raw.MultipartForm.File[key]
		if len(fhs) > 0 {
			files := make([]*File, len(fhs))
			for i, fh := range fhs {
				files[i] = NewFile(fh)
			}
			return files, nil
		}
	}

	if err := r.parseMultipartForm(32 << 20); err != nil {
		return nil, err
	}

	if r.multipartForm != nil && r.multipartForm.File != nil {
		fhs := r.multipartForm.File[key]
		files := make([]*File, len(fhs))
		for i, fh := range fhs {
			files[i] = NewFile(fh)
		}
		return files, nil
	}

	return nil, nil
}

// Body reads and caches the full request body, automatically decompressing if Content-Encoding is present.
func (r *Request) Body() []byte {
	r.bodyMu.Lock()
	defer r.bodyMu.Unlock()

	if r.bodyBuf != nil {
		if ce := r.Header(header.ContentEncoding); ce != "" && ce != "identity" {
			decompressed, err := compress.Decompress(ce, r.bodyBuf)
			if err == nil {
				r.bodyBuf = decompressed
			}
		}
		return r.bodyBuf
	}

	if r.raw != nil && r.raw.Body != nil {
		data, err := io.ReadAll(r.raw.Body)
		if err != nil {
			return nil
		}
		if ce := r.Header(header.ContentEncoding); ce != "" && ce != "identity" {
			decompressed, err := compress.Decompress(ce, data)
			if err == nil {
				data = decompressed
			}
		}
		r.bodyBuf = data
		return data
	}
	return nil
}

// BindJSON decodes the JSON request body into dest and executes automatic validation if dest implements Validatable.
func (r *Request) BindJSON(dest any) error {
	body := r.Body()
	if len(body) == 0 {
		return ErrEmptyRequestBody
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return ErrInvalidJSONPayload.WithCause(err)
	}

	if v, ok := dest.(Validatable); ok {
		if err := v.Validate(); err != nil {
			var domainErr DomainError
			if errors.As(err, &domainErr) {
				return domainErr
			}
			return ErrValidationFailed.WithMessage(err.Error())
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
