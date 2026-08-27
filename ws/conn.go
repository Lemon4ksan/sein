// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/lemon4ksan/foundation/silicon/simd"
)

// ApplyMask masks or unmasks payload in-place using the 4-octet masking key (RFC 6455 §5.3).
func ApplyMask(payload []byte, mask [4]byte) {
	if len(payload) == 0 {
		return
	}
	maskKey := binary.LittleEndian.Uint32(mask[:])
	simd.XORMask32(payload, maskKey)
}

// VectorApplyFastMask applies the 4-byte WebSocket XOR mask key to payload using hardware SIMD.
func VectorApplyFastMask(payload []byte, mask [4]byte) {
	ApplyMask(payload, mask)
}

// BuildFrameHeader serializes a WebSocket frame header (RFC 6455 §5.2).
func BuildFrameHeader(dst []byte, opcode byte, length int, compress, isClient bool) int {
	dst[0] = 0x80 | opcode
	if compress {
		dst[0] |= 0x40
	}

	dst[1] = 0
	if isClient {
		dst[1] = 0x80
	}

	switch {
	case length < 126:
		dst[1] |= byte(length)
		return 2

	case length <= 0xffff:
		dst[1] |= 126
		binary.BigEndian.PutUint16(dst[2:4], uint16(length))
		return 4

	default:
		dst[1] |= 127
		binary.BigEndian.PutUint64(dst[2:10], uint64(length))
		return 10
	}
}

// Conn represents an active, RFC 6455 compliant server-side WebSocket connection.
type Conn struct {
	conn      net.Conn
	br        *bufio.Reader
	bw        *bufio.Writer
	writeMu   sync.Mutex
	readMu    sync.Mutex
	isClosed  atomic.Bool
	maxMsgLen int64

	readDeadline  time.Time
	writeDeadline time.Time
}

// SetReadDeadline sets the deadline for future Read calls.
func (c *Conn) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the deadline for future Write calls.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return c.conn.SetWriteDeadline(t)
}

// SetMaxMessageSize sets the maximum permitted incoming message payload size in bytes.
func (c *Conn) SetMaxMessageSize(limit int64) {
	if limit > 0 {
		c.maxMsgLen = limit
	}
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// LocalAddr returns the local network address.
func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// ReadMessage reads the next complete data message (Text or Binary) from the WebSocket.
// Automatically responds to Ping control frames and handles Close frames.
func (c *Conn) ReadMessage() (int, []byte, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	var (
		messageType int
		messageBuf  []byte
	)

	for {
		if c.isClosed.Load() {
			return 0, nil, ErrConnectionClosed
		}

		// 1. Read 2-byte frame header
		var hdr [2]byte
		if _, err := io.ReadFull(c.br, hdr[:]); err != nil {
			return 0, nil, err
		}

		fin := (hdr[0] & 0x80) != 0
		rsv := hdr[0] & 0x70
		opcode := int(hdr[0] & 0x0F)
		masked := (hdr[1] & 0x80) != 0
		payloadLen := int64(hdr[1] & 0x7F)

		// RFC 6455 §5.2: RSV1-3 MUST be 0 unless extensions are negotiated
		if rsv != 0 {
			_ = c.CloseWithStatus(StatusProtocolError, "RSV bits must be 0")
			return 0, nil, ErrReservedBits
		}

		// RFC 6455 §5.1 & RFC 9931 §6.2: Client-to-server frames MUST be masked
		if !masked {
			_ = c.CloseWithStatus(StatusProtocolError, "client frame must be masked")
			return 0, nil, ErrMaskRequired
		}

		// RFC 6455 §5.5: Control frames MUST NOT be fragmented and MUST have payload <= 125 bytes
		if opcode >= 0x8 {
			if !fin {
				_ = c.CloseWithStatus(StatusProtocolError, "control frame must not be fragmented")
				return 0, nil, ErrFragmentedControl
			}

			if payloadLen > 125 {
				_ = c.CloseWithStatus(StatusProtocolError, "control frame payload exceeds 125 octets")
				return 0, nil, ErrControlFrameTooLarge
			}
		}

		// 2. Read extended payload length
		switch payloadLen {
		case 126:
			var ext [2]byte
			if _, err := io.ReadFull(c.br, ext[:]); err != nil {
				return 0, nil, err
			}

			payloadLen = int64(binary.BigEndian.Uint16(ext[:]))
			// RFC 6455 §5.2: Minimal encoding rule
			if payloadLen < 126 {
				_ = c.CloseWithStatus(StatusProtocolError, "payload length not minimally encoded")
				return 0, nil, ErrNonMinimalPayloadLength
			}
		case 127:
			var ext [8]byte
			if _, err := io.ReadFull(c.br, ext[:]); err != nil {
				return 0, nil, err
			}

			// RFC 6455 §5.2: Most significant bit MUST be 0
			if (ext[0] & 0x80) != 0 {
				_ = c.CloseWithStatus(StatusProtocolError, "invalid 64-bit payload length MSB")
				return 0, nil, ErrNonMinimalPayloadLength
			}

			payloadLen = int64(binary.BigEndian.Uint64(ext[:]))
			// RFC 6455 §5.2: Minimal encoding rule
			if payloadLen <= 65535 {
				_ = c.CloseWithStatus(StatusProtocolError, "payload length not minimally encoded")
				return 0, nil, ErrNonMinimalPayloadLength
			}
		}

		if payloadLen > c.maxMsgLen {
			_ = c.CloseWithStatus(StatusMessageTooBig, "message payload exceeded size limit")
			return 0, nil, ErrPayloadTooLarge
		}

		// 3. Read 4-byte mask key
		var maskKey [4]byte
		if _, err := io.ReadFull(c.br, maskKey[:]); err != nil {
			return 0, nil, err
		}

		// 4. Read payload
		payload := make([]byte, payloadLen)
		if payloadLen > 0 {
			if _, err := io.ReadFull(c.br, payload); err != nil {
				return 0, nil, err
			}

			// 5. Apply SIMD AVX2/NEON unmasking
			ApplyMask(payload, maskKey)
		}

		// Handle Control Frames (Ping, Pong, Close)
		switch opcode {
		case OpPing:
			_ = c.WritePong(payload)
			continue

		case OpPong:
			continue

		case OpClose:
			var (
				code   = StatusNormalClosure
				reason string
			)
			if payloadLen >= 2 {
				code = int(binary.BigEndian.Uint16(payload[:2]))
				if payloadLen > 2 {
					reason = string(payload[2:])
				}
			}

			_ = c.writeCloseFrame(code, reason)
			c.isClosed.Store(true)
			_ = c.conn.Close()

			return 0, nil, io.EOF

		case OpContinuation:
			if messageType == 0 {
				return 0, nil, ErrNotWebSocket
			}

			messageBuf = append(messageBuf, payload...)
			if fin {
				if messageType == OpText && !utf8.Valid(messageBuf) {
					_ = c.CloseWithStatus(StatusInvalidPayload, "invalid utf-8")
					return 0, nil, ErrInvalidUTF8
				}

				return messageType, messageBuf, nil
			}

		case OpText, OpBinary:
			if messageType == 0 {
				messageType = opcode
			}

			if fin {
				if opcode == OpText && !utf8.Valid(payload) {
					_ = c.CloseWithStatus(StatusInvalidPayload, "invalid utf-8")
					return 0, nil, ErrInvalidUTF8
				}

				return opcode, payload, nil
			}

			messageBuf = append(messageBuf, payload...)

		default:
			_ = c.CloseWithStatus(StatusProtocolError, "unknown opcode")
			return 0, nil, ErrNotWebSocket
		}
	}
}

// WriteMessage sends a WebSocket data message (Text or Binary) to the client.
// Server frames are NOT masked per RFC 6455 §5.1.
func (c *Conn) WriteMessage(messageType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.isClosed.Load() {
		return ErrConnectionClosed
	}

	var hdrBuf [10]byte

	n := BuildFrameHeader(hdrBuf[:], byte(messageType), len(data), false, false)

	if _, err := c.bw.Write(hdrBuf[:n]); err != nil {
		return err
	}

	if len(data) > 0 {
		if _, err := c.bw.Write(data); err != nil {
			return err
		}
	}

	return c.bw.Flush()
}

// WriteText sends a UTF-8 text message with zero memory allocations.
func (c *Conn) WriteText(text string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.isClosed.Load() {
		return ErrConnectionClosed
	}

	var hdrBuf [10]byte

	n := BuildFrameHeader(hdrBuf[:], OpText, len(text), false, false)

	if _, err := c.bw.Write(hdrBuf[:n]); err != nil {
		return err
	}

	if len(text) > 0 {
		if _, err := c.bw.WriteString(text); err != nil {
			return err
		}
	}

	return c.bw.Flush()
}

// WriteJSON serializes v into JSON and sends it as a Text message.
func (c *Conn) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	return c.WriteMessage(OpText, data)
}

// ReadJSON reads the next text message and decodes it into dest.
func (c *Conn) ReadJSON(dest any) error {
	_, data, err := c.ReadMessage()
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// WritePing sends a Ping control frame to the client.
func (c *Conn) WritePing(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.isClosed.Load() {
		return ErrConnectionClosed
	}

	var hdrBuf [10]byte

	n := BuildFrameHeader(hdrBuf[:], OpPing, len(data), false, false)
	if _, err := c.bw.Write(hdrBuf[:n]); err != nil {
		return err
	}

	if len(data) > 0 {
		if _, err := c.bw.Write(data); err != nil {
			return err
		}
	}

	return c.bw.Flush()
}

// WritePong sends a Pong control frame to the client.
func (c *Conn) WritePong(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.isClosed.Load() {
		return ErrConnectionClosed
	}

	var hdrBuf [10]byte

	n := BuildFrameHeader(hdrBuf[:], OpPong, len(data), false, false)
	if _, err := c.bw.Write(hdrBuf[:n]); err != nil {
		return err
	}

	if len(data) > 0 {
		if _, err := c.bw.Write(data); err != nil {
			return err
		}
	}

	return c.bw.Flush()
}

func (c *Conn) writeCloseFrame(code int, reason string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// RFC 6455 §7.4.1: Reserved status codes MUST NOT be set on the wire
	if code == StatusNoStatusRcvd || code == StatusAbnormalClosure || code == StatusTLSHandshake || code < 1000 ||
		code > 4999 {
		code = StatusNormalClosure
	}

	payloadLen := 2 + len(reason)
	payload := make([]byte, payloadLen)
	binary.BigEndian.PutUint16(payload[:2], uint16(code))
	copy(payload[2:], reason)

	var hdrBuf [10]byte

	n := BuildFrameHeader(hdrBuf[:], OpClose, payloadLen, false, false)
	_, _ = c.bw.Write(hdrBuf[:n])
	_, _ = c.bw.Write(payload)

	return c.bw.Flush()
}

// Close gracefully closes the WebSocket connection with StatusNormalClosure.
func (c *Conn) Close() error {
	return c.CloseWithStatus(StatusNormalClosure, "normal closure")
}

// CloseWithStatus closes the WebSocket connection with a custom status code and reason.
func (c *Conn) CloseWithStatus(code int, reason string) error {
	if c.isClosed.Swap(true) {
		return nil
	}

	_ = c.writeCloseFrame(code, reason)

	return c.conn.Close()
}
