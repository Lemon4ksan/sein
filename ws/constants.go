// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import "errors"

// RFC 6455 WebSocket Frame Opcodes (Section 5.2).
const (
	OpContinuation = 0x0
	OpText         = 0x1
	OpBinary       = 0x2
	OpClose        = 0x8
	OpPing         = 0x9
	OpPong         = 0xA
)

// RFC 6455 Close Status Codes (Section 7.4.1).
const (
	StatusNormalClosure   = 1000
	StatusGoingAway       = 1001
	StatusProtocolError   = 1002
	StatusUnsupportedData = 1003
	StatusNoStatusRcvd    = 1005
	StatusAbnormalClosure = 1006
	StatusInvalidPayload  = 1007
	StatusPolicyViolation = 1008
	StatusMessageTooBig   = 1009
	StatusMandatoryExt    = 1010
	StatusInternalError   = 1011
	StatusServiceRestart  = 1012
	StatusTryAgainLater   = 1013
	StatusBadGateway      = 1014
	StatusTLSHandshake    = 1015
)

// Standard WebSocket magic GUID per RFC 6455 Section 1.3.
const MagicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

var (
	ErrNotWebSocket          = errors.New("ws: not a websocket upgrade request")
	ErrMissingKey            = errors.New("ws: missing Sec-WebSocket-Key header")
	ErrUnsupportedVersion    = errors.New("ws: unsupported Sec-WebSocket-Version (expected 13)")
	ErrConnectionClosed      = errors.New("ws: connection closed")
	ErrMaskRequired         = errors.New("ws: client frame must be masked (RFC 6455 §5.1)")
	ErrReservedBits         = errors.New("ws: RSV bits must be 0 (RFC 6455 §5.2)")
	ErrControlFrameTooLarge = errors.New("ws: control frame payload exceeds 125 octets (RFC 6455 §5.5)")
	ErrFragmentedControl    = errors.New("ws: control frame must not be fragmented (RFC 6455 §5.5)")
	ErrInvalidUTF8             = errors.New("ws: invalid UTF-8 payload in text frame (RFC 6455 §5.6)")
	ErrPayloadTooLarge         = errors.New("ws: message payload exceeds max size (RFC 6455 §7.4.1)")
	ErrNonMinimalPayloadLength = errors.New("ws: payload length not encoded in minimal bytes (RFC 6455 §5.2)")
)
