// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/timekit"

	"github.com/lemon4ksan/sein/internal/fast/h1engine"
	"github.com/lemon4ksan/sein/internal/fast/h2engine"
	"github.com/lemon4ksan/sein/internal/fast/h3engine"
	"github.com/lemon4ksan/sein/internal/quic"
)

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
// serving HTTP/1.1, HTTP/2, and WebSockets over TCP, and HTTP/3 (QUIC) over UDP concurrently on the same port.
func (s *Server) ListenAndServeUniversal(addr, certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = "443"
	}

	s.mu.Lock()
	s.addr = addr
	s.altSvcHeader = fmt.Sprintf("h3=\":%s\"; ma=86400", port)
	s.mu.Unlock()

	// 1. TCP TLS Listener with ALPN (h2, http/1.1)
	tcpTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
	}

	var lc net.ListenConfig
	tcpLn, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.tcpLn = tcpLn
	s.mu.Unlock()

	// 2. UDP QUIC Listener with ALPN (h3)
	quicTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h3"},
	}

	quicConf := &quic.Config{
		EnableDatagrams: true,
	}

	quicLn, err := quic.ListenAddr(addr, quicTLS, quicConf)
	if err != nil {
		_ = tcpLn.Close()
		return err
	}

	s.mu.Lock()
	s.quicLn = quicLn
	s.mu.Unlock()

	errCh := make(chan error, 2)

	// Accept UDP QUIC connections
	go func() {
		for {
			conn, err := quicLn.Accept(context.Background())
			if err != nil {
				errCh <- err
				return
			}

			sc := h3engine.NewServerConn(conn, s.DispatchH3)
			go func() {
				_ = sc.Serve()
			}()
		}
	}()

	// Accept TCP TLS connections with ALPN demux
	go func() {
		tlsListener := tls.NewListener(tcpLn, tcpTLS)
		for {
			conn, err := tlsListener.Accept()
			if err != nil {
				errCh <- err
				return
			}

			go func(c net.Conn) {
				tlsConn, ok := c.(*tls.Conn)
				if !ok {
					_ = c.Close()
					return
				}

				if err := tlsConn.HandshakeContext(context.Background()); err != nil {
					_ = tlsConn.Close()
					return
				}

				proto := tlsConn.ConnectionState().NegotiatedProtocol
				if proto == "h2" {
					sc := h2engine.NewServerConn(tlsConn, s.DispatchH2)
					_ = sc.Serve()
				} else {
					connHandler := &h1engine.ConnHandler{
						Handler: s.dispatchH1,
					}
					_ = connHandler.ServeConn(tlsConn)
				}
			}(conn)
		}
	}()

	return <-errCh
}

// Shutdown gracefully shuts down all server listeners (TCP H1/H2 and UDP QUIC H3).
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var firstErr error
	if s.h1Server != nil {
		if err := s.h1Server.Shutdown(ctx); err != nil {
			firstErr = err
		}
	}

	if s.tcpLn != nil {
		if err := s.tcpLn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.tcpLn = nil
	}

	if s.quicLn != nil {
		if err := s.quicLn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.quicLn = nil
	}

	return firstErr
}

// Close gracefully closes the server.
func (s *Server) Close() error {
	return s.Shutdown(context.Background())
}

type statusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.statusCode = code
	sw.ResponseWriter.WriteHeader(code)
}

// ServeHTTP satisfies the standard http.Handler interface, enabling seamless interoperability with Go stdlib test recorders.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var params Params
	handler, allowHeader, redirectURL, redirectCode, status := s.resolveRoute(r.Method, r.URL.Path, &params)
	if redirectURL != "" {
		w.Header().Set(header.Location, redirectURL)
		w.WriteHeader(redirectCode)
		return
	}

	swWriter := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}
	req := NewRequest(r, &params)
	defer req.Release()

	if len(s.afterResponseHooks) > 0 || len(s.traceHooks) > 0 {
		sw := timekit.StartStopwatch()
		defer func() {
			s.triggerAfterResponse(req, swWriter.statusCode, sw.Elapsed())
		}()
	}

	if handler == nil {
		s.handleUnmatchedHTTP(swWriter, status, allowHeader)
		return
	}

	result, err := s.executePipeline(req, handler)
	if err != nil {
		s.writeError(swWriter, err)
		return
	}

	if responder, ok := result.(Responder); ok {
		if err := responder.WriteResponse(swWriter); err != nil {
			s.writeError(swWriter, ErrInternal("failed to write response", err))
		}

		return
	}

	_ = OK(result).WriteResponse(swWriter)
}

//go:noinline
func (s *Server) handleUnmatchedHTTP(w *statusWriter, status int, allowHeader string) {
	if status == http.StatusMethodNotAllowed {
		if allowHeader != "" {
			w.Header().Set(header.Allow, allowHeader)
		}
		s.writeError(w, NewHTTPError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}

	s.writeError(w, ErrNotFound("route not found"))
}
