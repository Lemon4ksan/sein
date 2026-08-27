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
	"net/netip"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/foundation/borrow"
	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/pool"

	"github.com/lemon4ksan/sein/internal/binder"
	"github.com/lemon4ksan/sein/internal/compress"
	"github.com/lemon4ksan/sein/internal/fast/h1engine"
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

type Request struct {
	ctx           context.Context
	method        string
	path          string
	routePattern  string
	query         string
	proto         string
	host          string
	remoteAddr    string
	bodyBuf       []byte
	h1Headers     *h1engine.Headers
	h1Req         *h1engine.Request
	raw           *http.Request
	multipartForm *multipart.Form
	params        Params
	scope         *borrow.Scope
	slots         [8]contextSlot
	slotCount     int
	overflow      map[reflect.Type]any
	bodyMu        sync.Mutex
	orphaned      bool
}

var requestStorage = pool.NewPerPStorage(func() *Request {
	return &Request{}
})

func acquireRequest() *Request {
	req := requestStorage.Get()
	req.reset()
	return req
}

func (r *Request) reset() {
	r.ctx = context.Background()
	r.method = ""
	r.path = ""
	r.routePattern = ""
	r.query = ""
	r.proto = ""
	r.host = ""
	r.remoteAddr = ""
	r.bodyBuf = nil
	r.h1Headers = nil
	r.h1Req = nil
	r.raw = nil
	r.multipartForm = nil
	r.params.Reset()
	r.slotCount = 0
	r.orphaned = false
	if r.overflow != nil {
		clear(r.overflow)
	}
}

// Detach prevents this Request from being returned to the memory pool upon completion (e.g. when abandoned to a background goroutine on timeout).
func (r *Request) Detach() {
	r.orphaned = true
}

// Scope returns the request-scoped lexical arena, guaranteed to be recycled with 0 GC allocations on request finish.
func (r *Request) Scope() *borrow.Scope {
	if r.scope == nil {
		r.scope = borrow.AcquireScope()
	}
	return r.scope
}

// Arena is an alias for Scope, providing a per-request bump allocator with zero GC overhead.
func (r *Request) Arena() *borrow.Scope {
	return r.Scope()
}

// AllocBytes allocates a zero-copy byte slice of the requested size out of the per-request arena.
func (r *Request) AllocBytes(size int) []byte {
	return r.Scope().AllocBytes(size).Bytes()
}

// AllocString clones a string into the contiguous per-request arena buffer without heap allocation.
func (r *Request) AllocString(s string) string {
	if len(s) == 0 {
		return ""
	}
	b := r.AllocBytes(len(s))
	copy(b, s)
	return bytesconv.B2S(b)
}

// Release returns the Request and its internal borrow arena to the sharded per-P memory pool.
func (r *Request) Release() {
	if r == nil || r.orphaned {
		return
	}
	if r.scope != nil {
		r.scope.Release()
		r.scope = nil
	}
	if r.multipartForm != nil {
		_ = r.multipartForm.RemoveAll()
		r.multipartForm = nil
	}
	r.reset()
	requestStorage.Put(r)
}

// NewRequest creates a Request wrapping a standard http.Request.
func NewRequest(r *http.Request, params ...*Params) *Request {
	req := acquireRequest()
	req.ctx = r.Context()
	req.raw = r
	req.method = r.Method
	req.remoteAddr = r.RemoteAddr
	req.proto = r.Proto
	req.host = r.Host
	if len(params) > 0 && params[0] != nil {
		req.params = *params[0]
	}
	if r.URL != nil {
		req.path = r.URL.Path
		req.query = r.URL.RawQuery
	}

	return req
}

// NewH1Request creates a Request wrapping a native zero-net/http h1.Request.
func NewH1Request(h1Req *h1engine.Request, params ...*Params) *Request {
	req := acquireRequest()
	req.method = h1Req.Method
	req.path = h1Req.Path
	req.query = h1Req.Query
	req.proto = h1Req.Proto
	req.host = h1Req.Host
	req.remoteAddr = h1Req.RemoteAddr
	req.bodyBuf = h1Req.Body
	req.h1Headers = &h1Req.Headers
	req.h1Req = h1Req
	if len(params) > 0 && params[0] != nil {
		req.params = *params[0]
	}

	return req
}

// NewH2Request creates a Request wrapping a native H2 stream request.
func NewH2Request(
	method, path, authority, remoteAddr string,
	rawHeaders http.Header,
	body []byte,
	params ...*Params,
) *Request {
	req := acquireRequest()
	req.method = method
	req.path = path
	req.proto = "HTTP/2.0"
	req.host = authority
	req.remoteAddr = remoteAddr
	req.bodyBuf = body
	if len(params) > 0 && params[0] != nil {
		req.params = *params[0]
	}
	if rawHeaders != nil {
		h := h1engine.NewHeadersWithCapacity(len(rawHeaders))
		for k, vv := range rawHeaders {
			for _, v := range vv {
				h.Set(k, v)
			}
		}

		req.h1Headers = &h
	}

	return req
}

// NewH3Request creates a Request wrapping a native H3 stream request.
func NewH3Request(
	method, path, authority, remoteAddr string,
	rawHeaders http.Header,
	body []byte,
	params ...*Params,
) *Request {
	req := acquireRequest()
	req.method = method
	req.path = path
	req.proto = "HTTP/3.0"
	req.host = authority
	req.remoteAddr = remoteAddr
	req.bodyBuf = body
	if len(params) > 0 && params[0] != nil {
		req.params = *params[0]
	}
	if rawHeaders != nil {
		h := h1engine.NewHeadersWithCapacity(len(rawHeaders))
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

// SetContext sets a new context on the request.
func (r *Request) SetContext(ctx context.Context) {
	r.ctx = ctx
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

// RoutePattern returns the registered route template pattern (e.g. "/users/:id").
// If the route is an unmatched 404 or unregistered path, it defaults to Path().
func (r *Request) RoutePattern() string {
	if r.routePattern != "" {
		return r.routePattern
	}

	return r.Path()
}

// SetRoutePattern manually sets the route pattern for custom request dispatching.
func (r *Request) SetRoutePattern(pattern string) {
	r.routePattern = pattern
}

// Proto returns the HTTP protocol version (e.g. "HTTP/1.1", "HTTP/2.0", "HTTP/3.0").
func (r *Request) Proto() string {
	if r.proto != "" {
		return r.proto
	}

	if r.raw != nil {
		return r.raw.Proto
	}

	return "HTTP/1.1"
}

// SetPath sets or rewrites the requested URL path.
func (r *Request) SetPath(path string) {
	r.path = path
	if r.raw != nil && r.raw.URL != nil {
		r.raw.URL.Path = path
	}
	if r.h1Req != nil {
		r.h1Req.Path = path
	}
}

// SetMethod sets or rewrites the HTTP request method.
func (r *Request) SetMethod(method string) {
	r.method = method
	if r.raw != nil {
		r.raw.Method = method
	}
	if r.h1Req != nil {
		r.h1Req.Method = method
	}
}

// SetQuery sets or rewrites the raw query string.
func (r *Request) SetQuery(query string) {
	r.query = query
	if r.raw != nil && r.raw.URL != nil {
		r.raw.URL.RawQuery = query
	}
	if r.h1Req != nil {
		r.h1Req.Query = query
	}
}

// Param retrieves a URL path parameter by name (e.g. "id" for "/users/:id").
func (r *Request) Param(name string) ParamValue {
	return ParamValue(r.params.Get(name))
}

// Params returns the underlying zero-alloc Params struct.
func (r *Request) Params() *Params {
	return &r.params
}

// ParamMap returns a copy of path parameters as a map for compatibility.
func (r *Request) ParamMap() map[string]string {
	return r.params.Map()
}

// Query retrieves a query parameter by key.
func (r *Request) Query(key string) ParamValue {
	if r.query != "" {
		for pair := range strings.SplitSeq(r.query, "&") {
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

// SetHeader sets or replaces a request header value.
func (r *Request) SetHeader(key, val string) {
	if r.h1Headers != nil {
		r.h1Headers.Set(key, val)
	}

	if r.raw != nil {
		r.raw.Header.Set(key, val)
	}
}

// DelHeader removes a request header.
func (r *Request) DelHeader(key string) {
	if r.h1Headers != nil {
		r.h1Headers.Del(key)
	}

	if r.raw != nil {
		r.raw.Header.Del(key)
	}
}

// Cookies parses and returns the HTTP cookies sent with the request.
func (r *Request) Cookies() []*http.Cookie {
	cookieHdr := r.Header(header.Cookie)
	if cookieHdr == "" {
		return nil
	}

	var cookies []*http.Cookie
	for k, v := range bytesconv.ScanPairs(cookieHdr, ';', '=') {
		// #nosec G124 -- Parsing incoming Cookie header
		cookies = append(cookies, &http.Cookie{
			Name:  strings.TrimSpace(k),
			Value: strings.TrimSpace(v),
		})
	}

	return cookies
}

// Bind ingests the request path parameters, query parameters, headers, and payload into dest using the precompiled binder.
func (r *Request) Bind(dest any) error {
	if dest == nil {
		return errors.New("sein: dest cannot be nil")
	}

	val := reflect.ValueOf(dest)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return errors.New("sein: dest must be a non-nil pointer")
	}

	adapter := newRequestAdapter(r)
	if err := binder.IngestType(adapter, val.Type().Elem(), dest); err != nil {
		return mapBinderError(err)
	}

	return nil
}


// BearerToken extracts the token from the "Authorization: Bearer <token>" header.
func (r *Request) BearerToken() (string, bool) {
	auth := r.Header(header.Authorization)

	prefix := header.ValueBearer + " "
	if token, ok :=strings.CutPrefix(auth, prefix); ok  {
		return strings.TrimSpace(token), true
	}

	return "", false
}

// DefaultTrustedProxies defines common private and loopback network ranges for trusted proxy resolution.
var DefaultTrustedProxies = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),    // IPv4 Loopback
	netip.MustParsePrefix("::1/128"),        // IPv6 Loopback
	netip.MustParsePrefix("10.0.0.0/8"),     // RFC 1918 Class A
	netip.MustParsePrefix("172.16.0.0/12"),  // RFC 1918 Class B
	netip.MustParsePrefix("192.168.0.0/16"), // RFC 1918 Class C
	netip.MustParsePrefix("169.254.0.0/16"), // Link-Local IPv4
	netip.MustParsePrefix("fe80::/10"),      // Link-Local IPv6
	netip.MustParsePrefix("fc00::/7"),       // Unique Local IPv6 (ULA)
}

// IP returns the real client IP address.
func (r *Request) IP() string {
	return r.ClientIP()
}

// IPs returns all IP addresses from the X-Forwarded-For chain in order.
func (r *Request) IPs() []string {
	fwd := r.Header(header.XForwardedFor)
	if fwd == "" {
		return nil
	}

	parts := strings.Split(fwd, ",")
	ips := make([]string, 0, len(parts))
	for _, p := range parts {
		ip := strings.TrimSpace(p)
		if ip != "" && net.ParseIP(ip) != nil {
			ips = append(ips, ip)
		}
	}

	return ips
}

// ClientIP returns the real client IP address, checking platform headers (CF-Connecting-IP, Fly-Client-IP, True-Client-IP, X-Real-IP),
// and safely parsing X-Forwarded-For right-to-left using [DefaultTrustedProxies] to prevent IP spoofing attacks.
func (r *Request) ClientIP() string {
	return r.ClientIPWithTrust(DefaultTrustedProxies)
}

// ClientIPWithTrust returns the real client IP address by traversing the X-Forwarded-For chain right-to-left,
// skipping any intermediate proxies matching the provided trusted IP prefixes.
func (r *Request) ClientIPWithTrust(trustedProxies []netip.Prefix) string {
	if cfIP := r.Header(header.CFConnectingIP); cfIP != "" {
		return strings.TrimSpace(cfIP)
	}

	if flyIP := r.Header("Fly-Client-IP"); flyIP != "" {
		return strings.TrimSpace(flyIP)
	}

	if trueIP := r.Header("True-Client-IP"); trueIP != "" {
		return strings.TrimSpace(trueIP)
	}

	if realIP := r.Header(header.XRealIP); realIP != "" {
		return strings.TrimSpace(realIP)
	}

	if fwd := r.Header(header.XForwardedFor); fwd != "" {
		var firstValidIP string
		remaining := fwd
		for len(remaining) > 0 {
			var item string
			lastComma := strings.LastIndexByte(remaining, ',')
			if lastComma == -1 {
				item = remaining
				remaining = ""
			} else {
				item = remaining[lastComma+1:]
				remaining = remaining[:lastComma]
			}

			rawIP := strings.TrimSpace(item)
			if rawIP == "" {
				continue
			}

			addr, err := netip.ParseAddr(rawIP)
			if err != nil {
				continue
			}

			if firstValidIP == "" {
				firstValidIP = rawIP
			}

			isTrusted := false
			for _, prefix := range trustedProxies {
				if prefix.Contains(addr) {
					isTrusted = true
					break
				}
			}

			if !isTrusted {
				return rawIP
			}
		}

		if firstValidIP != "" {
			return firstValidIP
		}
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

// RemoteAddr returns the raw remote network address (IP:port).
func (r *Request) RemoteAddr() string {
	if r.remoteAddr != "" {
		return r.remoteAddr
	}

	if r.raw != nil {
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

// Scheme returns the normalized request scheme ("https" or "http").
// It strictly validates X-Forwarded-Proto and Forwarded headers to prevent Open Redirect
// and header injection vulnerabilities.
func (r *Request) Scheme() string {
	if proto := r.Header(header.XForwardedProto); proto != "" {
		if strings.EqualFold(proto, "https") {
			return "https"
		}

		if strings.EqualFold(proto, "http") {
			return "http"
		}
	}

	if fwd := r.Header(header.Forwarded); fwd != "" {
		for _, part := range strings.Split(fwd, ";") {
			k, v, found := strings.Cut(strings.TrimSpace(part), "=")
			if found && strings.EqualFold(k, "proto") {
				v = strings.Trim(v, `"`)
				if strings.EqualFold(v, "https") {
					return "https"
				}

				if strings.EqualFold(v, "http") {
					return "http"
				}
			}
		}
	}

	if r.raw != nil {
		if r.raw.TLS != nil {
			return "https"
		}

		if r.raw.URL != nil && r.raw.URL.Scheme != "" {
			if strings.EqualFold(r.raw.URL.Scheme, "https") {
				return "https"
			}

			if strings.EqualFold(r.raw.URL.Scheme, "http") {
				return "http"
			}
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
		body := bytesconv.B2S(r.Body())
		for k, v := range bytesconv.ScanPairs(body, '&', '=') {
			if k == key {
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

	for k, v := range bytesconv.ScanPairs(cookieHeader, ';', '=') {
		if strings.TrimSpace(k) == name {
			return strings.TrimSpace(v), nil
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

// SaveUploadedFile streams an uploaded multipart file directly to dstPath on disk,
// automatically creating necessary parent directories with restricted 0750 permissions.
//
// Usage:
//
//	s.Post("/upload", func(req *sein.Request, _ struct{}) (any, error) {
//		file, err := req.FormFile("document")
//		if err != nil {
//			return nil, err
//		}
//		return "uploaded", req.SaveUploadedFile(file, "/data/uploads/"+file.Filename)
//	})
func (r *Request) SaveUploadedFile(file *File, dstPath string) error {
	if file == nil {
		return ErrBadRequest("nil file provided to SaveUploadedFile")
	}

	return file.SaveTo(dstPath)
}

// RawBody returns the raw, un-decompressed request payload bytes.
func (r *Request) RawBody() []byte {
	r.bodyMu.Lock()
	defer r.bodyMu.Unlock()

	if r.bodyBuf != nil {
		return r.bodyBuf
	}

	if r.raw != nil && r.raw.Body != nil {
		data, err := io.ReadAll(r.raw.Body)
		if err != nil {
			return nil
		}

		r.bodyBuf = data

		return data
	}

	return nil
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

// SetBody overrides the request payload buffer.
func (r *Request) SetBody(body []byte) {
	r.bodyMu.Lock()
	defer r.bodyMu.Unlock()

	r.bodyBuf = body
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

// Set stores a typed value in the request's flat inline context storage with 0 heap allocations.
//
// # Architectural Invariants: L1 CPU Cache Locality
//
// Stores up to 8 typed values in a contiguous inline array on the [Request] struct itself,
// allowing sub-nanosecond lookups directly in L1 CPU cache without map hashing overhead.
//
// # Example
//
//	sein.Set(req, &UserSession{UserID: 42, Role: "admin"})
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

// Get retrieves a typed value from the request's flat inline context storage (0 B/op).
//
// # Example
//
//	if session, ok := sein.Get[*UserSession](req); ok {
//	    log.Printf("Current user: %d", session.UserID)
//	}
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

// MustGet retrieves a typed value from request storage, panicking if the value was not set.
//
// # Example
//
//	session := sein.MustGet[*UserSession](req)
func MustGet[T any](r *Request) T {
	val, ok := Get[T](r)
	if !ok {
		panic("sein: requested type not found in request storage: " + reflect.TypeFor[T]().String())
	}

	return val
}
