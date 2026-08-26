// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// HeaderLen is the length of gRPC 5-byte frame header.
	HeaderLen = 5

	// DefaultMaxReceiveMsgSize is default 4MB limit.
	DefaultMaxReceiveMsgSize = 1024 * 1024 * 4

	// DefaultMaxSendMsgSize is default 4MB limit.
	DefaultMaxSendMsgSize = 1024 * 1024 * 4
)

var (
	// ErrMsgTooLarge indicates received message exceeds max configured limit.
	ErrMsgTooLarge = errors.New("grpc: received message larger than max allowed size")
)

// ReadMsg parses a 5-byte gRPC frame header and reads the message payload.
func ReadMsg(r io.Reader, maxMsgSize int) ([]byte, bool, error) {
	var header [HeaderLen]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, false, err
	}

	compressed := header[0] == 1
	msgLen := binary.BigEndian.Uint32(header[1:5])

	if maxMsgSize > 0 && int(msgLen) > maxMsgSize {
		return nil, false, fmt.Errorf("%w: %d > %d", ErrMsgTooLarge, msgLen, maxMsgSize)
	}

	if msgLen == 0 {
		return nil, compressed, nil
	}

	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, false, err
	}

	return buf, compressed, nil
}

// WriteMsg writes a 5-byte gRPC frame header followed by the message payload.
func WriteMsg(w io.Writer, data []byte, compressed bool) error {
	var header [HeaderLen]byte
	if compressed {
		header[0] = 1
	}

	binary.BigEndian.PutUint32(header[1:5], uint32(len(data)))

	totalLen := HeaderLen + len(data)
	buf := make([]byte, totalLen)
	copy(buf[:HeaderLen], header[:])
	copy(buf[HeaderLen:], data)

	_, err := w.Write(buf)

	return err
}
