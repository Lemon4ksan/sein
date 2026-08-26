// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socketio

import "errors"

var (
	// ErrServerClosed is returned when an operation is performed on a closed Socket.IO server.
	ErrServerClosed = errors.New("socketio: server is closed")

	// ErrSocketClosed is returned when attempting to emit or interact with a disconnected socket.
	ErrSocketClosed = errors.New("socketio: socket is closed")

	// ErrAckTimeout is returned when an acknowledgment was not received within the deadline.
	ErrAckTimeout = errors.New("socketio: acknowledgment timed out")

	// ErrInvalidPacket is returned when parsing a malformed or corrupt Socket.IO/Engine.IO packet.
	ErrInvalidPacket = errors.New("socketio: invalid packet payload")

	// ErrEmptyPacket is returned when an empty packet frame is encountered.
	ErrEmptyPacket = errors.New("socketio: empty packet")

	// ErrNamespaceNotFound is returned when an operation references an unregistered namespace.
	ErrNamespaceNotFound = errors.New("socketio: namespace not found")

	// ErrUnauthorized is returned when connection middleware rejects the handshake.
	ErrUnauthorized = errors.New("socketio: unauthorized connection")

	// ErrBinaryAttachmentMismatch is returned when expected binary attachments do not match received buffers.
	ErrBinaryAttachmentMismatch = errors.New("socketio: binary attachment count mismatch")

	// ErrPacketTooLarge is returned when payload length exceeds configured limits.
	ErrPacketTooLarge = errors.New("socketio: packet payload exceeded max size")
)
