// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/timekit"

	"github.com/lemon4ksan/sein/internal/fast/h1engine"
	"github.com/lemon4ksan/sein/internal/fast/h2engine"
	"github.com/lemon4ksan/sein/internal/fast/h3engine"
	"github.com/lemon4ksan/sein/internal/quic"
)

// Option configures a sein Server instance.
type Option func(s *Server)

// ErrorMapper translates arbitrary errors into typed DomainErrors.
type ErrorMapper func(err error) (DomainError, bool)

// AfterResponseHook is a lifecycle callback invoked asynchronously after an HTTP response has been flushed to the client.
type AfterResponseHook func(req *Request, statusCode int, duration time.Duration)

// TraceInfo encapsulates detailed execution metrics across each phase of an HTTP request lifecycle.
type TraceInfo struct {
	Method        string        `json:"method"`
	Path          string        `json:"path"`
	StatusCode    int           `json:"status_code"`
	ClientIP      string        `json:"client_ip"`
	TotalDuration time.Duration `json:"total_duration"`
}

// TraceHook is a callback invoked with granular request execution timings.
type TraceHook func(t *TraceInfo)

// Server represents a high-throughput, multi-protocol HTTP server engine supporting
// HTTP/1.1, HTTP/2, HTTP/3 (QUIC), and WebSockets on a single port with zero net/http overhead.
//
// # Architectural Context: Zero-Allocation Protocol Matrix
//
// Sein unifies modern IETF protocols into a single event-driven reactor. Incoming requests are routed
// via a zero-allocation Radix router directly to typed pure handlers without runtime reflection overhead.
//
// # Thread Safety
//
// 100% thread-safe for concurrent request execution. Configuration methods (e.g. [Server.Use], [Server.Get])
// should be called during server setup prior to calling [Server.Listen].
//
// # Example
//
//	srv := sein.New(
//	    sein.WithAddr(":8080"),
//	    sein.WithTrailingSlashRedirect(true),
//	)
//
//	srv.Get("/health", func(ctx context.Context) (string, error) {
//	    return "OK", nil
//	})
//
//	log.Fatal(srv.Listen(":8080"))
type Server struct {
	addr                   string
	router                 *Router
	middlewares            []Middleware
	errorMappers           []ErrorMapper
	afterResponseHooks     []AfterResponseHook
	traceHooks             []TraceHook
	resolvers              sync.Map
	h1Server               *h1engine.Server
	tcpLn                  net.Listener
	quicLn                 *quic.Listener
	altSvcHeader           string
	RedirectTrailingSlash  bool
	HandleMethodNotAllowed bool
	SkipUnmatchedRoutes    bool
	Prefork                bool
	noRouteHandler         RawHandler
	noMethodHandler        RawHandler
	trustedProxies         []*net.IPNet
	trustedPlatform        string
	AutoTLSDomains         []string
	AutoTLSCacheDir        string
	mu                     sync.Mutex
}

// WithAddr configures the default listening network address (e.g. ":8080" or "0.0.0.0:443").
func WithAddr(addr string) Option {
	return func(s *Server) {
		s.addr = addr
	}
}

// WithTrailingSlashRedirect configures whether requests with mismatched trailing slashes are automatically redirected (RFC 9110 §15.4.2).
func WithTrailingSlashRedirect(enabled bool) Option {
	return func(s *Server) {
		s.RedirectTrailingSlash = enabled
	}
}

// WithMethodNotAllowed configures whether 405 Method Not Allowed is automatically returned when a path exists for other HTTP verbs (RFC 9110 §15.5.6).
func WithMethodNotAllowed(enabled bool) Option {
	return func(s *Server) {
		s.HandleMethodNotAllowed = enabled
	}
}

// WithSkipUnmatchedRoutes configures whether global middlewares are bypassed for unmatched routes (404 / 405).
func WithSkipUnmatchedRoutes(enabled bool) Option {
	return func(s *Server) {
		s.SkipUnmatchedRoutes = enabled
	}
}

// WithPrefork enables high-load multi-process socket preforking on UNIX systems (`SO_REUSEPORT`).
func WithPrefork(enabled bool) Option {
	return func(s *Server) {
		s.Prefork = enabled
	}
}

// WithTrustedProxies configures trusted reverse proxy CIDRs/IPs for anti-spoofing [Request.ClientIP] resolution.
func WithTrustedProxies(proxies []string) Option {
	return func(s *Server) {
		_ = s.SetTrustedProxies(proxies)
	}
}

// WithTrustedPlatform sets a trusted platform header (e.g. "CF-Connecting-IP") for ClientIP extraction.
func WithTrustedPlatform(headerName string) Option {
	return func(s *Server) {
		s.SetTrustedPlatform(headerName)
	}
}

// WithAutoTLS configures zero-config automatic TLS certificate provisioning via ACME (Let's Encrypt / ZeroSSL).
func WithAutoTLS(domains ...string) Option {
	return func(s *Server) {
		s.AutoTLSDomains = append(s.AutoTLSDomains, domains...)
	}
}

// WithAutoTLSCacheDir configures the directory used to persist ACME certificates on disk.
func WithAutoTLSCacheDir(dir string) Option {
	return func(s *Server) {
		s.AutoTLSCacheDir = dir
	}
}

// New creates a new, fully initialized [Server] instance configured with options.
func New(opts ...Option) *Server {
	s := &Server{
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

// MapErrors registers multiple domain error mappings from a dictionary table at once.
func (s *Server) MapErrors(errorsMap Errors) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	for target, domainErr := range errorsMap {
		t := target
		d := domainErr

		s.errorMappers = append(s.errorMappers, func(err error) (DomainError, bool) {
			if errors.Is(err, t) {
				return d, true
			}

			return nil, false
		})
	}

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

// NoRoute registers a custom fallback handler for requests that match no registered routes (HTTP 404).
func (s *Server) NoRoute(handler RawHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.noRouteHandler = handler
}

// NoMethod registers a custom fallback handler for requests where the route path exists
// but the requested HTTP verb is unsupported (HTTP 405 Method Not Allowed).
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

// SetTrustedPlatform configures the server to trust client IP addresses from specific cloud platform headers.
func (s *Server) SetTrustedPlatform(platformHeader string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.trustedPlatform = platformHeader
}

// AfterResponse registers a lifecycle hook that executes after every completed HTTP response.
func (s *Server) AfterResponse(fn AfterResponseHook) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.afterResponseHooks = append(s.afterResponseHooks, fn)
	return s
}

// Trace registers a micro-tracing observer callback invoked after every completed request.
func (s *Server) Trace(fn TraceHook) *Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.traceHooks = append(s.traceHooks, fn)
	return s
}

func (s *Server) triggerAfterResponse(req *Request, statusCode int, duration time.Duration) {
	if len(s.afterResponseHooks) > 0 {
		for _, hook := range s.afterResponseHooks {
			hook(req, statusCode, duration)
		}
	}

	if len(s.traceHooks) > 0 {
		info := TraceInfo{
			Method:        req.Method(),
			Path:          req.Path(),
			StatusCode:    statusCode,
			ClientIP:      req.ClientIP(),
			TotalDuration: duration,
		}
		for _, hook := range s.traceHooks {
			hook(&info)
		}
	}
}

// PrintRoutes formats and returns an ASCII table representation of all registered routes.
func (s *Server) PrintRoutes() string {
	routes := s.Routes()
	if len(routes) == 0 {
		return "No routes registered."
	}

	var sb strings.Builder
	sb.WriteString("\n┌─────────┬────────────────────────────────────────────────────────┐\n")
	sb.WriteString("│ METHOD  │ PATH                                                   │\n")
	sb.WriteString("├─────────┼────────────────────────────────────────────────────────┤\n")

	for _, r := range routes {
		fmt.Fprintf(&sb, "│ %-7s │ %-54s │\n", r.Method, r.Path)
	}

	sb.WriteString("└─────────┴────────────────────────────────────────────────────────┘\n")

	return sb.String()
}

func (s *Server) resolveRoute(
	method, path string,
	params *Params,
) (handler RawHandler, pattern, allowHeader, redirectURL string, redirectCode, status int) {
	h, pat, found := s.router.Match(method, path, params)
	if found {
		return h, pat, "", "", 0, http.StatusOK
	}

	h, allowHeader, redirectURL, redirectCode, status = s.resolveRouteSlow(method, path)
	return h, "", allowHeader, redirectURL, redirectCode, status
}

//go:noinline
func (s *Server) resolveRouteSlow(
	method, path string,
) (handler RawHandler, allowHeader, redirectURL string, redirectCode, status int) {
	// 1. Check Trailing Slash Auto-Correction (RFC 9110 §15.4.2)
	if s.RedirectTrailingSlash {
		if altPath, ok := s.router.FindTrailingSlash(method, path); ok {
			code := http.StatusMovedPermanently
			if method != http.MethodGet && method != http.MethodHead {
				code = http.StatusTemporaryRedirect
			}

			return nil, "", altPath, code, code
		}
	}

	// 2. Check OPTIONS Preflight for CORS
	if method == http.MethodOptions && s.router.HasPath(path) {
		return func(req *Request) (any, error) {
			return NoContent(), nil
		}, "", "", 0, http.StatusOK
	}

	// 3. Check 405 Method Not Allowed (RFC 9110 §15.5.6)
	if s.HandleMethodNotAllowed {
		allowed := s.router.AllowedMethods(path)
		if len(allowed) > 0 {
			allowHdr := strings.Join(allowed, ", ")
			if s.noMethodHandler != nil {
				return s.noMethodHandler, allowHdr, "", 0, http.StatusMethodNotAllowed
			}

			return nil, allowHdr, "", 0, http.StatusMethodNotAllowed
		}
	}

	// 4. Check 404 NoRoute Custom Fallback
	if s.noRouteHandler != nil {
		return s.noRouteHandler, "", "", 0, http.StatusNotFound
	}

	return nil, "", "", 0, http.StatusNotFound
}

// DispatchH1 dispatches an incoming native H1 request directly through the server's routing and middleware pipeline.
func (s *Server) DispatchH1(h1Req *h1engine.Request, h1Res *h1engine.Response) error {
	return s.dispatchH1(h1Req, h1Res)
}

// dispatchH1 is the native zero-net/http request pipeline dispatcher.
func (s *Server) dispatchH1(h1Req *h1engine.Request, h1Res *h1engine.Response) error {
	var params Params
	handler, pattern, allowHeader, redirectURL, redirectCode, status := s.resolveRoute(h1Req.Method, h1Req.Path, &params)
	if redirectURL != "" {
		return s.serializeH1Result(h1Res, Redirect(redirectURL, redirectCode))
	}

	req := NewH1Request(h1Req, &params)
	req.routePattern = pattern
	defer req.Release()

	if len(s.afterResponseHooks) > 0 || len(s.traceHooks) > 0 {
		sw := timekit.StartStopwatch()
		defer func() {
			s.triggerAfterResponse(req, h1Res.StatusCode, sw.Elapsed())
		}()
	}

	if handler == nil {
		return s.handleUnmatchedH1(req, h1Req.Method, h1Req.Path, h1Res, status, allowHeader)
	}

	result, err := s.executePipeline(req, handler)
	if err != nil {
		s.writeH1Error(h1Res, err)
		return nil
	}

	if s.altSvcHeader != "" && h1Res.Headers.Get(header.AltSvc) == "" {
		h1Res.Headers.Set(header.AltSvc, s.altSvcHeader)
	}

	if direct, ok := result.(DirectH1Responder); ok {
		return direct.WriteToH1(h1Res)
	}

	return s.serializeH1Result(h1Res, result)
}

// DispatchH2 is the native zero-net/http HTTP/2 stream request dispatcher.
func (s *Server) DispatchH2(h2Req *h2engine.ServerRequest, h2Res *h2engine.ServerResponse) error {
	var params Params
	handler, pattern, allowHeader, redirectURL, redirectCode, status := s.resolveRoute(h2Req.Method, h2Req.Path, &params)
	if redirectURL != "" {
		return s.serializeH2Result(h2Res, Redirect(redirectURL, redirectCode))
	}

	req := NewH2Request(h2Req.Method, h2Req.Path, h2Req.Authority, h2Req.RemoteAddr, h2Req.Headers, h2Req.Body, &params)
	req.routePattern = pattern
	defer req.Release()

	if len(s.afterResponseHooks) > 0 || len(s.traceHooks) > 0 {
		sw := timekit.StartStopwatch()
		defer func() {
			s.triggerAfterResponse(req, h2Res.StatusCode, sw.Elapsed())
		}()
	}

	if handler == nil {
		return s.handleUnmatchedH2(h2Res, status, allowHeader)
	}

	result, err := s.executePipeline(req, handler)
	if err != nil {
		s.writeH2Error(h2Res, err)
		return nil
	}

	return s.serializeH2Result(h2Res, result)
}

// DispatchH3 is the native zero-net/http HTTP/3 stream request dispatcher.
func (s *Server) DispatchH3(h3Req *h3engine.ServerRequest, h3Res *h3engine.ServerResponse) error {
	var params Params
	handler, pattern, allowHeader, redirectURL, redirectCode, status := s.resolveRoute(h3Req.Method, h3Req.Path, &params)
	if redirectURL != "" {
		return s.serializeH3Result(h3Res, Redirect(redirectURL, redirectCode))
	}

	req := NewH3Request(h3Req.Method, h3Req.Path, h3Req.Authority, h3Req.RemoteAddr, h3Req.Headers, h3Req.Body, &params)
	req.routePattern = pattern
	defer req.Release()

	if len(s.afterResponseHooks) > 0 || len(s.traceHooks) > 0 {
		sw := timekit.StartStopwatch()
		defer func() {
			s.triggerAfterResponse(req, h3Res.StatusCode, sw.Elapsed())
		}()
	}

	if handler == nil {
		return s.handleUnmatchedH3(h3Res, status, allowHeader)
	}

	result, err := s.executePipeline(req, handler)
	if err != nil {
		s.writeH3Error(h3Res, err)
		return nil
	}

	return s.serializeH3Result(h3Res, result)
}

func (s *Server) executePipeline(req *Request, handler RawHandler) (any, error) {
	if len(s.middlewares) == 0 {
		return handler(req)
	}

	h := handler
	for _, m := range slices.Backward(s.middlewares) {
		h = m(h)
	}

	return h(req)
}

//go:noinline
func (s *Server) handleUnmatchedH1(req *Request, origMethod, origPath string, h1Res *h1engine.Response, status int, allowHeader string) error {
	if !s.SkipUnmatchedRoutes && len(s.middlewares) > 0 {
		h := func(r *Request) (any, error) {
			return s.resolveUnmatched(r, origMethod, origPath, h1Res, status, allowHeader)
		}
		for _, m := range slices.Backward(s.middlewares) {
			h = m(h)
		}
		result, err := h(req)
		if err != nil {
			s.writeH1Error(h1Res, err)
			return nil
		}
		return s.serializeH1Result(h1Res, result)
	}

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

//go:noinline
func (s *Server) handleUnmatchedH2(h2Res *h2engine.ServerResponse, status int, allowHeader string) error {
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

//go:noinline
func (s *Server) handleUnmatchedH3(h3Res *h3engine.ServerResponse, status int, allowHeader string) error {
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

func (s *Server) resolveUnmatched(r *Request, origMethod, origPath string, h1Res *h1engine.Response, status int, allowHeader string) (any, error) {
	if r.Path() != origPath || r.Method() != origMethod {
		r.params.Reset()
		newHandler, newPattern, newAllow, newRedir, newRedirCode, newStatus := s.resolveRoute(r.Method(), r.Path(), &r.params)
		if newRedir != "" {
			return Redirect(newRedir, newRedirCode), nil
		}
		if newHandler != nil {
			r.routePattern = newPattern
			return newHandler(r)
		}
		if newStatus == http.StatusMethodNotAllowed {
			if newAllow != "" && h1Res != nil {
				h1Res.Headers.Set(header.Allow, newAllow)
			}
			return nil, NewHTTPError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		}
		return nil, ErrNotFound("route not found")
	}

	if status == http.StatusMethodNotAllowed {
		if allowHeader != "" && h1Res != nil {
			h1Res.Headers.Set(header.Allow, allowHeader)
		}
		return nil, NewHTTPError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}

	return nil, ErrNotFound("route not found")
}
