// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein/internal/fast/h2engine"
	"github.com/lemon4ksan/sein/internal/fast/h3engine"
	"github.com/lemon4ksan/sein/internal/fast/h1engine"
	"github.com/lemon4ksan/sein/internal/quic"
)

// Option configures a sein Server instance.
type Option func(s *Server)

// ErrorMapper translates arbitrary errors into typed DomainErrors.
type ErrorMapper func(err error) (DomainError, bool)

// Server represents a high-throughput, multi-protocol HTTP server engine supporting
// HTTP/1.1, HTTP/2, HTTP/3 (QUIC), and WebSockets with zero net/http overhead.
//
// Concurrency: Server is safe for concurrent initialization and request dispatching.
type Server struct {
	addr                   string
	router                 *Router
	middlewares            []Middleware
	errorMappers           []ErrorMapper
	h1Server               *h1engine.Server
	RedirectTrailingSlash  bool
	HandleMethodNotAllowed bool
	SkipUnmatchedRoutes    bool
	noRouteHandler         RawHandler
	noMethodHandler        RawHandler
	trustedProxies         []*net.IPNet
	trustedPlatform        string
	mu                     sync.Mutex
}

// WithAddr configures the listening network address (e.g. ":8080" or "127.0.0.1:443").
func WithAddr(addr string) Option {
	return func(s *Server) {
		s.addr = addr
	}
}

// WithTrailingSlashRedirect configures whether requests with mismatched trailing slashes are automatically redirected.
func WithTrailingSlashRedirect(enabled bool) Option {
	return func(s *Server) {
		s.RedirectTrailingSlash = enabled
	}
}

// WithMethodNotAllowed configures whether 405 Method Not Allowed is returned when path exists on other verbs.
func WithMethodNotAllowed(enabled bool) Option {
	return func(s *Server) {
		s.HandleMethodNotAllowed = enabled
	}
}

// WithSkipUnmatchedRoutes configures whether global middlewares are bypassed for unmatched routes (404/405).
func WithSkipUnmatchedRoutes(enabled bool) Option {
	return func(s *Server) {
		s.SkipUnmatchedRoutes = enabled
	}
}

// New creates a new sein Server instance with production defaults.
func New(opts ...Option) *Server {
	s := &Server{
		addr:                   ":8080",
		router:                 NewRouter(),
		RedirectTrailingSlash:  true,
		HandleMethodNotAllowed: true,
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

// NoRoute registers a custom fallback handler for requests that match no registered routes (HTTP 404).
//
// Usage:
//
//	server.NoRoute(func(req *sein.Request) (any, error) {
//		return sein.StatusWith(404, map[string]string{
//			"error": "custom 404 not found",
//		}, nil), nil
//	})
func (s *Server) NoRoute(handler RawHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.noRouteHandler = handler
}

// NoMethod registers a custom fallback handler for requests where the route path exists
// but the requested HTTP verb is unsupported (HTTP 405 Method Not Allowed).
//
// Usage:
//
//	server.NoMethod(func(req *sein.Request) (any, error) {
//		return sein.StatusWith(405, map[string]string{
//			"error": "method not allowed",
//		}, nil), nil
//	})
func (s *Server) NoMethod(handler RawHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.noMethodHandler = handler
}

// Routes returns an immutable snapshot list of all registered route patterns and methods in this server.
func (s *Server) Routes() []RouteInfo {
	return s.router.Routes()
}

// SetTrustedProxies configures a list of trusted reverse proxy IP addresses or CIDR subnets.
// When configured, client IP resolution inspects X-Forwarded-For headers only when the request
// originates from one of the trusted proxy networks.
//
// Usage:
//
//	err := server.SetTrustedProxies([]string{"127.0.0.1", "10.0.0.0/8", "192.168.0.0/16"})
func (s *Server) SetTrustedProxies(proxies []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	parsed := make([]*net.IPNet, 0, len(proxies))
	for _, p := range proxies {
		if strings.Contains(p, "/") {
			_, ipNet, err := net.ParseCIDR(p)
			if err != nil {
				return err
			}

			parsed = append(parsed, ipNet)
		} else {
			ip := net.ParseIP(p)
			if ip == nil {
				return fmt.Errorf("sein: invalid trusted proxy IP %q", p)
			}

			var mask net.IPMask
			if ip.To4() != nil {
				mask = net.CIDRMask(32, 32)
			} else {
				mask = net.CIDRMask(128, 128)
			}

			parsed = append(parsed, &net.IPNet{IP: ip, Mask: mask})
		}
	}

	s.trustedProxies = parsed

	return nil
}

// SetTrustedPlatform configures the server to trust client IP addresses from specific cloud platform headers
// (such as "CF-Connecting-IP" for Cloudflare or "Fly-Client-IP" for Fly.io).
//
// Usage:
//
//	server.SetTrustedPlatform("CF-Connecting-IP")
func (s *Server) SetTrustedPlatform(platformHeader string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.trustedPlatform = platformHeader
}

// MountRaw registers a low-level RawHandler on the specified HTTP method and route pattern.
func (s *Server) MountRaw(method, pattern string, handler RawHandler, mw ...Middleware) {
	s.registerRoute(method, pattern, handler, mw...)
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

func (s *Server) resolveRoute(
	method, path string,
) (handler RawHandler, params map[string]string, allowHeader, redirectURL string, redirectCode, status int) {
	h, p, found := s.router.Match(method, path)
	if found {
		return h, p, "", "", 0, http.StatusOK
	}

	// 1. Check Trailing Slash Auto-Correction (RFC 9110 §15.4.2)
	if s.RedirectTrailingSlash {
		if altPath, ok := s.router.FindTrailingSlash(method, path); ok {
			code := http.StatusMovedPermanently
			if method != http.MethodGet && method != http.MethodHead {
				code = http.StatusTemporaryRedirect
			}

			return nil, nil, "", altPath, code, code
		}
	}

	// 2. Check OPTIONS Preflight for CORS
	if method == http.MethodOptions && s.router.HasPath(path) {
		return func(req *Request) (any, error) {
			return NoContent(), nil
		}, nil, "", "", 0, http.StatusOK
	}

	// 3. Check 405 Method Not Allowed (RFC 9110 §15.5.6)
	if s.HandleMethodNotAllowed {
		allowed := s.router.AllowedMethods(path)
		if len(allowed) > 0 {
			allowHdr := strings.Join(allowed, ", ")
			if s.noMethodHandler != nil {
				return s.noMethodHandler, nil, allowHdr, "", 0, http.StatusMethodNotAllowed
			}

			return nil, nil, allowHdr, "", 0, http.StatusMethodNotAllowed
		}
	}

	// 4. Check 404 NoRoute Custom Fallback
	if s.noRouteHandler != nil {
		return s.noRouteHandler, nil, "", "", 0, http.StatusNotFound
	}

	return nil, nil, "", "", 0, http.StatusNotFound
}

// dispatchH1 is the native zero-net/http request pipeline dispatcher.
func (s *Server) dispatchH1(h1Req *h1engine.Request, h1Res *h1engine.Response) error {
	handler, params, allowHeader, redirectURL, redirectCode, status := s.resolveRoute(h1Req.Method, h1Req.Path)
	if redirectURL != "" {
		res := Redirect(redirectURL, redirectCode)
		return s.serializeH1Result(h1Res, res)
	}

	if handler == nil {
		if status == http.StatusMethodNotAllowed {
			if allowHeader != "" {
				h1Res.Headers.Set(header.Allow, allowHeader)
			}

			s.writeH1Error(h1Res, NewHTTPError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed"))

			return nil
		}

		s.writeH1Error(h1Res, ErrNotFound("route not found"))

		return nil
	}

	req := NewH1Request(h1Req, params)
	defer req.Release()

	// Wrap in global middlewares unless SkipUnmatchedRoutes is enabled on 404/405
	finalHandler := handler
	if !s.SkipUnmatchedRoutes || (status != http.StatusNotFound && status != http.StatusMethodNotAllowed) {
		for _, v := range slices.Backward(s.middlewares) {
			finalHandler = v(finalHandler)
		}
	}

	// Execute handler
	result, err := finalHandler(req)
	if err != nil {
		s.writeH1Error(h1Res, err)
		return nil
	}

	if direct, ok := result.(DirectH1Responder); ok {
		return direct.WriteToH1(h1Res)
	}

	return s.serializeH1Result(h1Res, result)
}

func (s *Server) serializeH1Result(res *h1engine.Response, result any) error {
	res.StatusCode = http.StatusOK

	switch v := result.(type) {
	case nil:
		return nil
	case []byte:
		if res.Headers.Get(header.ContentType) == "" {
			res.Headers.Set(header.ContentType, header.MIMEApplicationOctetStream)
		}

		res.Body = append(res.Body, v...)

		return nil

	case string:
		if res.Headers.Get(header.ContentType) == "" {
			res.Headers.Set(header.ContentType, header.MIMETextPlainCharsetUTF8)
		}

		res.Body = append(res.Body, v...)

		return nil

	default:
		if res.Headers.Get(header.ContentType) == "" {
			res.Headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)
		}

		data, err := json.Marshal(v)
		if err != nil {
			return err
		}

		res.Body = append(res.Body, data...)

		return nil
	}
}

type errorResponse struct {
	Status  int            `json:"status"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (s *Server) writeH1Error(res *h1engine.Response, err error) {
	for _, mapper := range s.errorMappers {
		if mapped, ok := mapper(err); ok {
			err = mapped
			break
		}
	}

	var (
		resp       errorResponse
		definedErr DefinedError
		domainErr  DomainError
		httpErr    HTTPError
	)

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

	res.StatusCode = resp.Status
	res.Headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)

	data, _ := json.Marshal(resp)
	res.Body = data
}

// DispatchH2 is the native zero-net/http HTTP/2 stream request dispatcher.
func (s *Server) DispatchH2(h2Req *h2engine.ServerRequest, h2Res *h2engine.ServerResponse) error {
	handler, params, allowHeader, redirectURL, redirectCode, status := s.resolveRoute(h2Req.Method, h2Req.Path)
	if redirectURL != "" {
		res := Redirect(redirectURL, redirectCode)
		return s.serializeH2Result(h2Res, res)
	}

	if handler == nil {
		if status == http.StatusMethodNotAllowed {
			if h2Res.Headers == nil {
				h2Res.Headers = make(http.Header)
			}

			if allowHeader != "" {
				h2Res.Headers.Set(header.Allow, allowHeader)
			}

			s.writeH2Error(h2Res, NewHTTPError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed"))

			return nil
		}

		s.writeH2Error(h2Res, ErrNotFound("route not found"))

		return nil
	}

	req := NewH2Request(h2Req.Method, h2Req.Path, h2Req.Authority, h2Req.RemoteAddr, h2Req.Headers, h2Req.Body, params)
	defer req.Release()

	// Wrap in global middlewares unless SkipUnmatchedRoutes is enabled on 404/405
	finalHandler := handler
	if !s.SkipUnmatchedRoutes || (status != http.StatusNotFound && status != http.StatusMethodNotAllowed) {
		for _, v := range slices.Backward(s.middlewares) {
			finalHandler = v(finalHandler)
		}
	}

	// Execute handler
	result, err := finalHandler(req)
	if err != nil {
		s.writeH2Error(h2Res, err)
		return nil
	}

	return s.serializeH2Result(h2Res, result)
}

func (s *Server) serializeH2Result(res *h2engine.ServerResponse, result any) error {
	res.StatusCode = http.StatusOK

	if holder, ok := result.(ResponseHolder); ok {
		res.StatusCode = holder.StatusCode()
		if res.StatusCode == 0 {
			res.StatusCode = http.StatusOK
		}

		res.Headers = holder.ResponseHeaders()
		if res.Headers == nil {
			res.Headers = make(http.Header)
		}

		body := holder.ResponseBody()
		switch b := body.(type) {
		case nil:
			return nil
		case []byte:
			res.Body = b
			return nil
		case string:
			res.Body = []byte(b)
			return nil
		default:
			data, err := json.Marshal(b)
			if err != nil {
				return err
			}

			res.Body = data
			if res.Headers.Get(header.ContentType) == "" {
				res.Headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)
			}

			return nil
		}
	}

	if res.Headers == nil {
		res.Headers = make(http.Header)
	}

	switch v := result.(type) {
	case nil:
		return nil
	case []byte:
		if res.Headers.Get(header.ContentType) == "" {
			res.Headers.Set(header.ContentType, header.MIMEApplicationOctetStream)
		}

		res.Body = append(res.Body, v...)

		return nil

	case string:
		if res.Headers.Get(header.ContentType) == "" {
			res.Headers.Set(header.ContentType, header.MIMETextPlainCharsetUTF8)
		}

		res.Body = append(res.Body, v...)

		return nil

	default:
		if res.Headers.Get(header.ContentType) == "" {
			res.Headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)
		}

		data, err := json.Marshal(v)
		if err != nil {
			return err
		}

		res.Body = append(res.Body, data...)

		return nil
	}
}

func (s *Server) writeH2Error(res *h2engine.ServerResponse, err error) {
	for _, mapper := range s.errorMappers {
		if mapped, ok := mapper(err); ok {
			err = mapped
			break
		}
	}

	var (
		resp       errorResponse
		definedErr DefinedError
		domainErr  DomainError
		httpErr    HTTPError
	)

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

	if res.Headers == nil {
		res.Headers = make(http.Header)
	}

	res.StatusCode = resp.Status
	res.Headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)

	data, _ := json.Marshal(resp)
	res.Body = data
}

// DispatchH3 is the native zero-net/http HTTP/3 stream request dispatcher.
func (s *Server) DispatchH3(h3Req *h3engine.ServerRequest, h3Res *h3engine.ServerResponse) error {
	handler, params, allowHeader, redirectURL, redirectCode, status := s.resolveRoute(h3Req.Method, h3Req.Path)
	if redirectURL != "" {
		res := Redirect(redirectURL, redirectCode)
		return s.serializeH3Result(h3Res, res)
	}

	if handler == nil {
		if status == http.StatusMethodNotAllowed {
			if h3Res.Headers == nil {
				h3Res.Headers = make(http.Header)
			}

			if allowHeader != "" {
				h3Res.Headers.Set(header.Allow, allowHeader)
			}

			s.writeH3Error(h3Res, NewHTTPError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed"))

			return nil
		}

		s.writeH3Error(h3Res, ErrNotFound("route not found"))

		return nil
	}

	req := NewH3Request(h3Req.Method, h3Req.Path, h3Req.Authority, h3Req.RemoteAddr, h3Req.Headers, h3Req.Body, params)
	defer req.Release()

	// Wrap in global middlewares unless SkipUnmatchedRoutes is enabled on 404/405
	finalHandler := handler
	if !s.SkipUnmatchedRoutes || (status != http.StatusNotFound && status != http.StatusMethodNotAllowed) {
		for _, v := range slices.Backward(s.middlewares) {
			finalHandler = v(finalHandler)
		}
	}

	// Execute handler
	result, err := finalHandler(req)
	if err != nil {
		s.writeH3Error(h3Res, err)
		return nil
	}

	return s.serializeH3Result(h3Res, result)
}

func (s *Server) serializeH3Result(res *h3engine.ServerResponse, result any) error {
	res.StatusCode = http.StatusOK

	if holder, ok := result.(ResponseHolder); ok {
		res.StatusCode = holder.StatusCode()
		if res.StatusCode == 0 {
			res.StatusCode = http.StatusOK
		}

		res.Headers = holder.ResponseHeaders()
		if res.Headers == nil {
			res.Headers = make(http.Header)
		}

		body := holder.ResponseBody()
		switch b := body.(type) {
		case nil:
			return nil
		case []byte:
			res.Body = b
			return nil
		case string:
			res.Body = []byte(b)
			return nil
		default:
			data, err := json.Marshal(b)
			if err != nil {
				return err
			}

			res.Body = data
			if res.Headers.Get(header.ContentType) == "" {
				res.Headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)
			}

			return nil
		}
	}

	if res.Headers == nil {
		res.Headers = make(http.Header)
	}

	switch v := result.(type) {
	case nil:
		return nil
	case []byte:
		if res.Headers.Get(header.ContentType) == "" {
			res.Headers.Set(header.ContentType, header.MIMEApplicationOctetStream)
		}

		res.Body = append(res.Body, v...)

		return nil

	case string:
		if res.Headers.Get(header.ContentType) == "" {
			res.Headers.Set(header.ContentType, header.MIMETextPlainCharsetUTF8)
		}

		res.Body = append(res.Body, v...)

		return nil

	default:
		if res.Headers.Get(header.ContentType) == "" {
			res.Headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)
		}

		data, err := json.Marshal(v)
		if err != nil {
			return err
		}

		res.Body = append(res.Body, data...)

		return nil
	}
}

func (s *Server) writeH3Error(res *h3engine.ServerResponse, err error) {
	for _, mapper := range s.errorMappers {
		if mapped, ok := mapper(err); ok {
			err = mapped
			break
		}
	}

	var (
		resp       errorResponse
		definedErr DefinedError
		domainErr  DomainError
		httpErr    HTTPError
	)

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

	if res.Headers == nil {
		res.Headers = make(http.Header)
	}

	res.StatusCode = resp.Status
	res.Headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)

	data, _ := json.Marshal(resp)
	res.Body = data
}

// ServeHTTP satisfies the standard http.Handler interface, enabling seamless interoperability with Go stdlib test recorders.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler, params, allowHeader, redirectURL, redirectCode, status := s.resolveRoute(r.Method, r.URL.Path)
	if redirectURL != "" {
		w.Header().Set(header.Location, redirectURL)
		w.WriteHeader(redirectCode)
		return
	}

	if handler == nil {
		if status == http.StatusMethodNotAllowed {
			if allowHeader != "" {
				w.Header().Set(header.Allow, allowHeader)
			}

			s.writeError(w, NewHTTPError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed"))

			return
		}

		s.writeError(w, ErrNotFound("route not found"))

		return
	}

	req := NewRequest(r, params)
	defer req.Release()

	// Wrap in global middlewares unless SkipUnmatchedRoutes is enabled on 404/405
	finalHandler := handler
	if !s.SkipUnmatchedRoutes || (status != http.StatusNotFound && status != http.StatusMethodNotAllowed) {
		for _, v := range slices.Backward(s.middlewares) {
			finalHandler = v(finalHandler)
		}
	}

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

	_ = OK(result).WriteResponse(w)
}

func (s *Server) writeError(w http.ResponseWriter, err error) {
	for _, mapper := range s.errorMappers {
		if mapped, ok := mapper(err); ok {
			err = mapped
			break
		}
	}

	var (
		resp       errorResponse
		definedErr DefinedError
		domainErr  DomainError
		httpErr    HTTPError
	)

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

// Serve starts the native H1 zero-net/http server on the provided net.Listener.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	s.h1Server = h1engine.NewServer(s.dispatchH1)
	s.mu.Unlock()

	return s.h1Server.Serve(ln)
}

// ListenAndServe starts the native H1 zero-net/http server listening on the configured address.
func (s *Server) ListenAndServe() error {
	addr := s.addr
	if addr == "" {
		addr = ":8080"
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return err
	}

	return s.Serve(ln)
}

// ListenAndServeTLS starts listening on s.addr with TLS using native H1 engine.
func (s *Server) ListenAndServeTLS(certFile, keyFile string) error {
	s.mu.Lock()
	s.h1Server = h1engine.NewServer(s.dispatchH1)
	s.h1Server.Addr = s.addr
	s.mu.Unlock()

	return s.h1Server.ListenAndServeTLS(certFile, keyFile)
}

// Listen starts listening on the specified address.
func (s *Server) Listen(addr string) error {
	s.addr = addr
	return s.ListenAndServe()
}

// ListenAndServeQUIC starts the native HTTP/3 server over UDP using TLS.
func (s *Server) ListenAndServeQUIC(addr, certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h3"},
	}

	quicConf := &quic.Config{
		EnableDatagrams: true,
	}

	ln, err := quic.ListenAddr(addr, tlsConf, quicConf)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()

	for {
		conn, err := ln.Accept(context.Background())
		if err != nil {
			return err
		}

		sc := h3engine.NewServerConn(conn, s.DispatchH3)
		go func() {
			_ = sc.Serve()
		}()
	}
}

// ListenAndServeUniversal starts the unified multi-protocol engine on port addr (e.g. :443)
// serving HTTP/1.1, HTTP/2, and WebSockets over TCP, and HTTP/3 over UDP.
func (s *Server) ListenAndServeUniversal(addr, certFile, keyFile string) error {
	errCh := make(chan error, 2)

	// 1. Start HTTP/3 over UDP
	go func() {
		errCh <- s.ListenAndServeQUIC(addr, certFile, keyFile)
	}()

	// 2. Start HTTP/1.1 & HTTP/2 over TCP
	go func() {
		errCh <- s.ListenAndServeTLS(certFile, keyFile)
	}()

	return <-errCh
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.h1Server != nil {
		return s.h1Server.Shutdown(ctx)
	}

	return nil
}

// Close gracefully closes the server.
func (s *Server) Close() error {
	return s.Shutdown(context.Background())
}
