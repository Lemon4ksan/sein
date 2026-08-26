// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socketio

import (
	"encoding/json"
	"time"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/sein"
)

// SocketID uniquely identifies an active client connection socket.
type SocketID = string

// Room represents a grouping channel for broadcasting messages.
type Room = string

// HandshakeData contains metadata extracted from the initial HTTP/WebSocket handshake and Socket.IO connect frame.
type HandshakeData struct {
	// Headers contains all incoming HTTP request headers.
	Headers map[string]string `json:"headers"`

	// Query contains URL query parameters parsed from the connection URI.
	Query map[string]string `json:"query"`

	// Auth contains the arbitrary authentication credentials payload supplied in the SIO Connect packet.
	Auth json.RawMessage `json:"auth,omitempty"`

	// RemoteAddr is the network address of the connecting client.
	RemoteAddr string `json:"remoteAddr"`

	// IssuedAt is the timestamp when the session was established.
	IssuedAt time.Time `json:"issuedAt"`

	// Secure indicates whether the handshake was negotiated over TLS/WSS.
	Secure bool `json:"secure"`
}

// AckFunc is a callback passed to event handlers to acknowledge the receipt of an event with response data.
type AckFunc func(args ...any)

// EventHandler is a callback invoked when a matching event is received from the client.
type EventHandler func(args []json.RawMessage)

// Middleware is a connection interceptor invoked during namespace connection establishment.
type Middleware func(socket *Socket, next func(err error))

// Config configures the Socket.IO and Engine.IO server behavior.
type Config struct {
	// PingInterval defines the frequency of heartbeat ping packets (default: 25s).
	PingInterval time.Duration

	// PingTimeout defines the duration after which an inactive connection is severed (default: 20s).
	PingTimeout time.Duration

	// MaxPayload defines the maximum allowed message payload size in bytes (default: 1MB).
	MaxPayload int64

	// ConnectTimeout defines the deadline for a client to complete SIO namespace handshake (default: 15s).
	ConnectTimeout time.Duration

	// CheckOrigin returns true if the incoming request Origin is permitted (default: allow all).
	CheckOrigin func(req *sein.Request) bool

	// Adapter customizes the room membership and broadcasting backend (default: in-memory adapter).
	Adapter Adapter
}

// ResolveDefaults applies robust production defaults to zero-valued config fields.
func (cfg *Config) ResolveDefaults() {
	cfg.PingInterval = generic.Coalesce(cfg.PingInterval, 25*time.Second)
	cfg.PingTimeout = generic.Coalesce(cfg.PingTimeout, 20*time.Second)
	if cfg.MaxPayload <= 0 {
		cfg.MaxPayload = 1024 * 1024 // 1 MB
	}
	cfg.ConnectTimeout = generic.Coalesce(cfg.ConnectTimeout, 15*time.Second)
	if cfg.CheckOrigin == nil {
		cfg.CheckOrigin = func(req *sein.Request) bool { return true }
	}
	if cfg.Adapter == nil {
		cfg.Adapter = NewMemoryAdapter()
	}
}

// BroadcastOptions specifies routing filters for emitted packets.
type BroadcastOptions struct {
	// Rooms contains target rooms for delivery. Empty means broadcast to the whole namespace.
	Rooms []Room

	// Except contains socket IDs or rooms to exclude from delivery.
	Except []string

	// Volatile indicates that packets may be dropped if client buffers are full or socket is not ready.
	Volatile bool
}
