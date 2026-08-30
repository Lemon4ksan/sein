// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1engine

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var ErrServerClosed = errors.New("h1: server is closed")

// Server is a zero-net/http HTTP/1.1 listener and connection orchestrator.
type Server struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	MaxBodySize  int64
	TLSConfig    *tls.Config
	Handler      HandlerFunc

	listener net.Listener
	conns    map[net.Conn]struct{}
	mu       sync.Mutex
	closed   atomic.Bool
	active   sync.WaitGroup
	quit     chan struct{}
}

// NewServer creates a new H1 Server.
func NewServer(handler HandlerFunc) *Server {
	return &Server{
		Handler: handler,
		conns:   make(map[net.Conn]struct{}),
		quit:    make(chan struct{}),
	}
}

func (s *Server) trackConn(c net.Conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conns == nil {
		s.conns = make(map[net.Conn]struct{})
	}

	if add {
		s.conns[c] = struct{}{}
	} else {
		delete(s.conns, c)
	}
}

// Serve accepts incoming connections on ln and starts the worker pipeline.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	if s.closed.Load() {
		s.mu.Unlock()
		return ErrServerClosed
	}

	s.listener = ln
	s.mu.Unlock()

	connHandler := &ConnHandler{
		ReadTimeout:  s.ReadTimeout,
		WriteTimeout: s.WriteTimeout,
		IdleTimeout:  s.IdleTimeout,
		MaxBodySize:  s.MaxBodySize,
		Handler:      s.Handler,
	}

	var tempDelay time.Duration

	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.closed.Load() {
				return ErrServerClosed
			}

			if _, ok := errors.AsType[net.Error](err); ok {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}

				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}

				time.Sleep(tempDelay)

				continue
			}

			return err
		}

		tempDelay = 0

		s.active.Add(1)

		s.trackConn(conn, true)
		go func(c net.Conn) {
			defer s.active.Done()
			defer s.trackConn(c, false)

			_ = connHandler.ServeConn(c)
		}(conn)
	}
}

// ListenAndServe starts listening on s.Addr (or :8080 default) and serves H1 connections.
func (s *Server) ListenAndServe() error {
	addr := s.Addr
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

// ListenAndServeTLS starts listening on s.Addr with TLS.
func (s *Server) ListenAndServeTLS(certFile, keyFile string) error {
	addr := s.Addr
	if addr == "" {
		addr = ":8443"
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	config := s.TLSConfig
	if config == nil {
		config = &tls.Config{
			NextProtos: []string{"http/1.1"},
		}
	} else {
		config = config.Clone()
	}

	config.Certificates = []tls.Certificate{cert}

	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return err
	}

	return s.Serve(tls.NewListener(ln, config))
}

// Shutdown gracefully stops accepting new connections and waits for active connections to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	s.mu.Lock()
	if s.listener != nil {
		_ = s.listener.Close()
	}

	for c := range s.conns {
		_ = c.SetDeadline(time.Now())
	}

	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.active.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
