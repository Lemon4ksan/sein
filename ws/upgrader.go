// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"crypto/sha1"
	"encoding/base64"
	"strings"
	"time"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/sein"
)

// Upgrader configures WebSocket connection handshake and limits.
type Upgrader struct {
	// HandshakeTimeout specifies the deadline for completing the handshake.
	HandshakeTimeout time.Duration
	// ReadBufferSize is the size of the I/O read buffer in bytes (default: 4096).
	ReadBufferSize int
	// WriteBufferSize is the size of the I/O write buffer in bytes (default: 4096).
	WriteBufferSize int
	// Subprotocols specifies the server's supported subprotocols in order of preference.
	Subprotocols []string
	// CheckOrigin returns true if the request Origin is accepted (default: accepts all origins).
	CheckOrigin func(req *sein.Request) bool
}

// Option configures an Upgrader.
type Option func(*Upgrader)

// WithCheckOrigin sets the origin validation callback.
func WithCheckOrigin(fn func(req *sein.Request) bool) Option {
	return func(u *Upgrader) {
		u.CheckOrigin = fn
	}
}

// WithSubprotocols sets supported subprotocols.
func WithSubprotocols(subprotocols ...string) Option {
	return func(u *Upgrader) {
		u.Subprotocols = subprotocols
	}
}

// ComputeAcceptKey computes the Sec-WebSocket-Accept hash per RFC 6455 §4.2.2 with stack allocation.
func ComputeAcceptKey(challengeKey string) string {
	var input [64]byte
	n := copy(input[:], challengeKey)
	n += copy(input[n:], MagicGUID)

	sum := sha1.Sum(input[:n])

	var acceptKey [28]byte
	base64.StdEncoding.Encode(acceptKey[:], sum[:])
	return string(acceptKey[:])
}

// Upgrade upgrades the incoming HTTP/1.1 request to a WebSocket connection.
func Upgrade(req *sein.Request, opts ...Option) (*Conn, error) {
	upgrader := Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     func(r *sein.Request) bool { return true },
	}
	for _, opt := range opts {
		opt(&upgrader)
	}

	// 1. Verify Upgrade and Connection headers
	if !strings.EqualFold(req.Header(header.Upgrade), header.ValueWebSocket) {
		return nil, ErrNotWebSocket
	}

	connHdr := req.Header(header.Connection)
	if !strings.Contains(strings.ToLower(connHdr), "upgrade") {
		return nil, ErrNotWebSocket
	}

	// 2. Verify Sec-WebSocket-Version == 13
	version := req.Header("Sec-WebSocket-Version")
	if version != "13" {
		return nil, ErrUnsupportedVersion
	}

	// 3. Verify Sec-WebSocket-Key
	clientKey := strings.TrimSpace(req.Header("Sec-WebSocket-Key"))
	if clientKey == "" {
		return nil, ErrMissingKey
	}

	// 4. Validate Origin
	if upgrader.CheckOrigin != nil && !upgrader.CheckOrigin(req) {
		return nil, sein.ErrForbidden("websocket origin rejected")
	}

	// 5. Compute Sec-WebSocket-Accept
	acceptKey := ComputeAcceptKey(clientKey)

	// 6. Hijack underlying connection
	netConn, rw, err := req.Hijack()
	if err != nil {
		return nil, err
	}

	// 7. Write 101 Switching Protocols response
	var sb strings.Builder
	sb.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	sb.WriteString("Upgrade: websocket\r\n")
	sb.WriteString("Connection: Upgrade\r\n")
	sb.WriteString("Sec-WebSocket-Accept: ")
	sb.WriteString(acceptKey)
	sb.WriteString("\r\n")

	// Negotiate subprotocol if requested
	if len(upgrader.Subprotocols) > 0 {
		clientProtocols := req.Header("Sec-WebSocket-Protocol")
		if clientProtocols != "" {
			for _, cp := range strings.Split(clientProtocols, ",") {
				cp = strings.TrimSpace(cp)
				for _, sp := range upgrader.Subprotocols {
					if strings.EqualFold(cp, sp) {
						sb.WriteString("Sec-WebSocket-Protocol: ")
						sb.WriteString(sp)
						sb.WriteString("\r\n")
						break
					}
				}
			}
		}
	}

	sb.WriteString("\r\n")

	_, err = rw.WriteString(sb.String())
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = netConn.Close()
		return nil, err
	}

	wsConn := &Conn{
		conn:      netConn,
		br:        rw.Reader,
		bw:        rw.Writer,
		maxMsgLen: 32 << 20, // 32MB default
	}

	return wsConn, nil
}
