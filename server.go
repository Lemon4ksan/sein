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

	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein/internal/fast/h1engine"
	"github.com/lemon4ksan/sein/internal/fast/h2engine"
	"github.com/lemon4ksan/sein/internal/fast/h3engine"
	"github.com/lemon4ksan/sein/internal/quic"
)

// Option configures a sein Server instance.
type Option func(s *Server)

// ErrorMapper translates arbitrary errors into typed DomainErrors.
type ErrorMapper func(err error) (DomainError, bool)

// Server represents a high-throughput, multi-protocol HTTP server engine supporting
// HTTP/1.1, HTTP/2, HTTP/3 (QUIC), and WebSockets with zero net/http overhead.
type Server struct {
	addr                   string
	router                 *Router
	middlewares            []Middleware
	errorMappers           []ErrorMapper
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

// WithPrefork enables highload multi-process preforking on UNIX systems.
func WithPrefork(enabled bool) Option {
	return func(s *Server) {
		s.Prefork = enabled
	}
}

// WithTrustedProxies configures trusted proxy CIDRs/IPs for secure ClientIP extraction.
func WithTrustedProxies(proxies []string) Option {
	return func(s *Server) {
		_ = s.SetTrustedProxies(proxies)
	}
}

// WithTrustedPlatform sets a trusted vendor header (e.g. "CF-Connecting-IP") for ClientIP extraction.
func WithTrustedPlatform(headerName string) Option {
	return func(s *Server) {
		s.SetTrustedPlatform(headerName)
	}
}

// New creates a new, fully initialized Sein server instance.
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
) (handler RawHandler, allowHeader, redirectURL string, redirectCode, status int) {
	h, found := s.router.Match(method, path, params)
	if found {
		return h, "", "", 0, http.StatusOK
	}

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

// dispatchH1 is the native zero-net/http request pipeline dispatcher.
func (s *Server) dispatchH1(h1Req *h1engine.Request, h1Res *h1engine.Response) error {
	var params Params
	handler, allowHeader, redirectURL, redirectCode, status := s.resolveRoute(h1Req.Method, h1Req.Path, &params)
	if redirectURL != "" {
		res := Redirect(redirectURL, redirectCode)
		return s.serializeH1Result(h1Res, res)
	}

	if handler == nil && s.SkipUnmatchedRoutes {
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

	req := NewH1Request(h1Req, &params)
	defer req.Release()

	origPath := h1Req.Path
	origMethod := h1Req.Method

	baseHandler := handler
	if baseHandler == nil {
		baseHandler = func(r *Request) (any, error) {
			if r.Path() != origPath || r.Method() != origMethod {
				r.params.Reset()
				newHandler, newAllow, newRedir, newRedirCode, newStatus := s.resolveRoute(r.Method(), r.Path(), &r.params)
				if newRedir != "" {
					return Redirect(newRedir, newRedirCode), nil
				}

				if newHandler != nil {
					return newHandler(r)
				}

				if newStatus == http.StatusMethodNotAllowed {
					if newAllow != "" {
						h1Res.Headers.Set(header.Allow, newAllow)
					}

					return nil, NewHTTPError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
				}
			}

			if status == http.StatusMethodNotAllowed {
				if allowHeader != "" {
					h1Res.Headers.Set(header.Allow, allowHeader)
				}

				return nil, NewHTTPError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			}

			return nil, ErrNotFound("route not found")
		}
	} else {
		actualHandler := baseHandler
		baseHandler = func(r *Request) (any, error) {
			if r.Path() != origPath || r.Method() != origMethod {
				r.params.Reset()
				newHandler, newAllow, newRedir, newRedirCode, newStatus := s.resolveRoute(r.Method(), r.Path(), &r.params)
				if newRedir != "" {
					return Redirect(newRedir, newRedirCode), nil
				}

				if newHandler != nil {
					return newHandler(r)
				}

				if newStatus == http.StatusMethodNotAllowed {
					if newAllow != "" {
						h1Res.Headers.Set(header.Allow, newAllow)
					}

					return nil, NewHTTPError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
				}

				return nil, ErrNotFound("route not found")
			}

			return actualHandler(r)
		}
	}

	finalHandler := baseHandler
	for _, v := range slices.Backward(s.middlewares) {
		finalHandler = v(finalHandler)
	}

	// Execute handler
	result, err := finalHandler(req)
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
	handler, allowHeader, redirectURL, redirectCode, status := s.resolveRoute(h2Req.Method, h2Req.Path, &params)
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

	req := NewH2Request(h2Req.Method, h2Req.Path, h2Req.Authority, h2Req.RemoteAddr, h2Req.Headers, h2Req.Body, &params)
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

// DispatchH3 is the native zero-net/http HTTP/3 stream request dispatcher.
func (s *Server) DispatchH3(h3Req *h3engine.ServerRequest, h3Res *h3engine.ServerResponse) error {
	var params Params
	handler, allowHeader, redirectURL, redirectCode, status := s.resolveRoute(h3Req.Method, h3Req.Path, &params)
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

	req := NewH3Request(h3Req.Method, h3Req.Path, h3Req.Authority, h3Req.RemoteAddr, h3Req.Headers, h3Req.Body, &params)
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
