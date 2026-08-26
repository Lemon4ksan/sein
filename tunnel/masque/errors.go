// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package masque

import "errors"

var (
	// ErrServerClosed is returned when operations are performed on a closed server.
	ErrServerClosed = errors.New("sein/masque: server is closed")

	// ErrNoRouteToHost is returned when an outbound packet has no matching client session or route.
	ErrNoRouteToHost = errors.New("sein/masque: no route to host")

	// ErrHandshakeFailed indicates the connect-ip proxy request failed or returned a non-200/101 status code.
	ErrHandshakeFailed = errors.New("sein/masque: connect-ip handshake failed")

	// ErrInvalidCapsule indicates a corrupted or truncated Capsule Protocol frame was received.
	ErrInvalidCapsule = errors.New("sein/masque: invalid capsule format")

	// ErrInvalidURITemplate indicates that the provided MASQUE URI template is malformed.
	ErrInvalidURITemplate = errors.New("sein/masque: invalid uri template")

	// ErrUnsupportedHTTPVersion indicates that connect-ip was attempted on an unsupported transport.
	ErrUnsupportedHTTPVersion = errors.New("sein/masque: unsupported http version for connect-ip")

	// ErrEmptyAddressRequest indicates an ADDRESS_REQUEST capsule contained zero requested addresses.
	ErrEmptyAddressRequest = errors.New("sein/masque: address request capsule cannot be empty")

	// ErrUnhandledProtocol indicates that an IP protocol handler is not registered in the protocol VTable.
	ErrUnhandledProtocol = errors.New("sein/masque: unhandled ip protocol")

	// ErrInvalidIPHeader indicates that an IP packet header is truncated or malformed.
	ErrInvalidIPHeader = errors.New("sein/masque: invalid ip packet header")

	// ErrMTUTooSmall indicates that requested MTU is below protocol minimums.
	ErrMTUTooSmall = errors.New("sein/masque: mtu below protocol minimum")
)
