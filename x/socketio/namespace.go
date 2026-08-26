// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socketio

import (
	"slices"
	"sync"
)

// Namespace represents a distinct communication multiplexing endpoint with its own middleware chain, rooms, and sockets.
type Namespace struct {
	name        string
	server      *Server
	adapter     Adapter
	middlewares []Middleware
	mwMu        sync.RWMutex

	sockets sync.Map // map[SocketID]*Socket

	connectHandlers []func(s *Socket)
	handlersMu      sync.RWMutex
}

func newNamespace(name string, server *Server, adapter Adapter) *Namespace {
	if name == "" || name[0] != '/' {
		name = "/" + name
	}

	return &Namespace{
		name:    name,
		server:  server,
		adapter: adapter,
	}
}

// Name returns the namespace path (e.g. "/", "/chat").
func (nsp *Namespace) Name() string {
	return nsp.name
}

// Use adds connection interceptor middlewares to this namespace.
func (nsp *Namespace) Use(mw ...Middleware) {
	nsp.mwMu.Lock()
	defer nsp.mwMu.Unlock()

	nsp.middlewares = append(nsp.middlewares, mw...)
}

// OnConnect registers a callback invoked when a new socket successfully connects and passes all middlewares.
func (nsp *Namespace) OnConnect(fn func(s *Socket)) {
	nsp.handlersMu.Lock()
	defer nsp.handlersMu.Unlock()

	nsp.connectHandlers = append(nsp.connectHandlers, fn)
}

// On registers event listeners on the namespace. "connection" or "connect" registers a connection handler.
func (nsp *Namespace) On(event string, fn any) {
	if event == "connection" || event == "connect" {
		if handler, ok := fn.(func(s *Socket)); ok {
			nsp.OnConnect(handler)
		}
	}
}

// Sockets returns a snapshot of all active sockets currently connected to this namespace.
func (nsp *Namespace) Sockets() []*Socket {
	var list []*Socket
	nsp.sockets.Range(func(key, value any) bool {
		list = append(list, value.(*Socket))
		return true
	})
	return list
}

// Socket looks up an active socket in this namespace by its ID.
func (nsp *Namespace) Socket(id SocketID) *Socket {
	val, ok := nsp.sockets.Load(id)
	if !ok {
		return nil
	}
	return val.(*Socket)
}

// Broadcast returns a BroadcastOperator targeting all sockets in this namespace.
func (nsp *Namespace) Broadcast() *BroadcastOperator {
	return NewBroadcastOperator(nsp)
}

// To returns a BroadcastOperator targeting specific rooms within this namespace.
func (nsp *Namespace) To(rooms ...Room) *BroadcastOperator {
	return NewBroadcastOperator(nsp).To(rooms...)
}

// In is an alias for To.
func (nsp *Namespace) In(rooms ...Room) *BroadcastOperator {
	return nsp.To(rooms...)
}

// Except returns a BroadcastOperator targeting the namespace while excluding specific rooms or sockets.
func (nsp *Namespace) Except(except ...string) *BroadcastOperator {
	return NewBroadcastOperator(nsp).Except(except...)
}

// Emit broadcasts an event and arguments to all connected sockets in this namespace.
func (nsp *Namespace) Emit(event string, args ...any) error {
	return nsp.Broadcast().Emit(event, args...)
}

// DisconnectSockets disconnects all sockets in this namespace.
func (nsp *Namespace) DisconnectSockets(closeUnderlying bool) {
	nsp.Broadcast().DisconnectSockets(closeUnderlying)
}

func (nsp *Namespace) registerSocket(s *Socket) {
	nsp.sockets.Store(s.id, s)
}

func (nsp *Namespace) removeSocket(id SocketID) {
	nsp.sockets.Delete(id)
}

func (nsp *Namespace) triggerConnect(s *Socket) {
	nsp.handlersMu.RLock()
	handlers := slices.Clone(nsp.connectHandlers)
	nsp.handlersMu.RUnlock()

	for _, h := range handlers {
		go h(s)
	}
}

func (nsp *Namespace) runMiddlewares(s *Socket, done func(err error)) {
	nsp.mwMu.RLock()
	middlewares := slices.Clone(nsp.middlewares)
	nsp.mwMu.RUnlock()

	if len(middlewares) == 0 {
		done(nil)
		return
	}

	var idx int
	var next func(err error)
	next = func(err error) {
		if err != nil {
			done(err)
			return
		}
		if idx >= len(middlewares) {
			done(nil)
			return
		}
		curr := middlewares[idx]
		idx++
		curr(s, next)
	}

	next(nil)
}
