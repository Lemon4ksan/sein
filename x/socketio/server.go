// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socketio

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/ws"
)

// Option configures a Socket.IO Server instance.
type Option func(*Server)

// WithPingInterval sets the Engine.IO heartbeat ping interval.
func WithPingInterval(d time.Duration) Option {
	return func(s *Server) {
		s.config.PingInterval = d
	}
}

// WithPingTimeout sets the Engine.IO heartbeat ping timeout.
func WithPingTimeout(d time.Duration) Option {
	return func(s *Server) {
		s.config.PingTimeout = d
	}
}

// WithMaxPayload sets the maximum permitted incoming packet size.
func WithMaxPayload(size int64) Option {
	return func(s *Server) {
		s.config.MaxPayload = size
	}
}

// WithCheckOrigin configures the Origin verification callback.
func WithCheckOrigin(fn func(req *sein.Request) bool) Option {
	return func(s *Server) {
		s.config.CheckOrigin = fn
	}
}

// WithAdapter configures a custom room and broadcast Adapter.
func WithAdapter(a Adapter) Option {
	return func(s *Server) {
		s.config.Adapter = a
	}
}

// WithConnectTimeout sets the deadline for handshake completion.
func WithConnectTimeout(d time.Duration) Option {
	return func(s *Server) {
		s.config.ConnectTimeout = d
	}
}

// Server is a high-performance Socket.IO v5 / Engine.IO v4 server designed for the sein framework.
type Server struct {
	config     Config
	namespaces sync.Map // map[string]*Namespace
	sessions   sync.Map // map[string]*engineSession
	closed     atomic.Bool
}

// NewServer instantiates a new Socket.IO server with the provided options.
func NewServer(opts ...Option) *Server {
	s := &Server{}
	for _, opt := range opts {
		opt(s)
	}
	s.config.ResolveDefaults()

	// Initialize default root namespace
	s.Of("/")

	return s
}

// Of retrieves or lazily instantiates a namespace under the specified path.
func (s *Server) Of(nsp string) *Namespace {
	if nsp == "" || nsp[0] != '/' {
		nsp = "/" + nsp
	}

	if val, ok := s.namespaces.Load(nsp); ok {
		return val.(*Namespace)
	}

	newNsp := newNamespace(nsp, s, s.config.Adapter)
	actual, _ := s.namespaces.LoadOrStore(nsp, newNsp)
	return actual.(*Namespace)
}

// OnConnect registers a connection listener on the root "/" namespace.
func (s *Server) OnConnect(fn func(socket *Socket)) {
	s.Of("/").OnConnect(fn)
}

// On registers an event listener on the root "/" namespace.
func (s *Server) On(event string, fn any) {
	s.Of("/").On(event, fn)
}

// Use adds a connection middleware to the root "/" namespace.
func (s *Server) Use(mw ...Middleware) {
	s.Of("/").Use(mw...)
}

// To returns a BroadcastOperator targeting specific rooms in the root "/" namespace.
func (s *Server) To(rooms ...Room) *BroadcastOperator {
	return s.Of("/").To(rooms...)
}

// In is an alias for To.
func (s *Server) In(rooms ...Room) *BroadcastOperator {
	return s.Of("/").In(rooms...)
}

// Except returns a BroadcastOperator excluding specific rooms or sockets in the root "/" namespace.
func (s *Server) Except(except ...string) *BroadcastOperator {
	return s.Of("/").Except(except...)
}

// Emit broadcasts an event and arguments to all connected sockets in the root "/" namespace.
func (s *Server) Emit(event string, args ...any) error {
	return s.Of("/").Emit(event, args...)
}

// Sockets returns all active sockets in the root "/" namespace.
func (s *Server) Sockets() []*Socket {
	return s.Of("/").Sockets()
}

// Mount satisfies sein.Module, mounting Socket.IO HTTP & WebSocket endpoints onto the given Group.
func (s *Server) Mount(g *sein.Group) {
	handler := s.Handler()

	sein.Handle(g, "GET", "", handler)
	sein.Handle(g, "GET", "/", handler)
	sein.Handle(g, "GET", "/*path", handler)
	sein.Handle(g, "POST", "", handler)
	sein.Handle(g, "POST", "/", handler)
	sein.Handle(g, "POST", "/*path", handler)
}

// Handler returns a raw sein.RawHandler that can be registered on any route.
func (s *Server) Handler() sein.RawHandler {
	return func(req *sein.Request) (any, error) {
		return nil, s.ServeHTTP(req)
	}
}

// ServeHTTP handles an incoming HTTP request, performing Origin verification and WebSocket protocol upgrade.
func (s *Server) ServeHTTP(req *sein.Request) error {
	if s.closed.Load() {
		return ErrServerClosed
	}

	if s.config.CheckOrigin != nil && !s.config.CheckOrigin(req) {
		return sein.ErrForbidden("socketio: origin rejected")
	}

	wsConn, err := ws.Upgrade(req, ws.WithCheckOrigin(s.config.CheckOrigin))
	if err != nil {
		return fmt.Errorf("socketio: websocket upgrade failed: %w", err)
	}

	handshake := s.extractHandshake(req)
	session := newEngineSession(s, wsConn, handshake)
	s.sessions.Store(session.sid, session)

	ctx, cancel := context.WithTimeout(req.Context(), s.config.ConnectTimeout)
	defer cancel()

	if err := session.start(ctx); err != nil {
		_ = session.Close("session start failed")
		s.sessions.Delete(session.sid)
		return err
	}

	return nil
}

func (s *Server) extractHandshake(req *sein.Request) HandshakeData {
	hs := HandshakeData{
		Headers:    make(map[string]string),
		Query:      make(map[string]string),
		RemoteAddr: req.RemoteAddr(),
		IssuedAt:   time.Now(),
		Secure:     req.Scheme() == "https",
	}

	if raw := req.Raw(); raw != nil {
		for k, v := range raw.Header {
			if len(v) > 0 {
				hs.Headers[k] = v[0]
			}
		}
		if raw.URL != nil {
			for k, v := range raw.URL.Query() {
				if len(v) > 0 {
					hs.Query[k] = v[0]
				}
			}
		}
	} else {
		for _, k := range []string{"user-agent", "cookie", "authorization", "origin", "sec-websocket-key", "sec-websocket-version"} {
			if v := req.Header(k); v != "" {
				hs.Headers[k] = v
			}
		}
	}

	return hs
}

// Close gracefully terminates the Socket.IO server and shuts down all active client sessions.
func (s *Server) Close() error {
	if s.closed.CompareAndSwap(false, true) {
		s.sessions.Range(func(key, value any) bool {
			session := value.(*engineSession)
			_ = session.Close("server shutdown")
			s.sessions.Delete(key)
			return true
		})
	}
	return nil
}
