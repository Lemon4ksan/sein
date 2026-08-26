// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package masque

import (
	"encoding/binary"
	"errors"
)

// EncodeVarintSlice encodes v into b using QUIC variable-length integer encoding (RFC 9000 §16 / RFC 9297 §1.1).
func EncodeVarintSlice(v uint64, b []byte) int {
	switch {
	case v < 1<<6:
		b[0] = byte(v)
		return 1
	case v < 1<<14:
		binary.BigEndian.PutUint16(b[:2], uint16(v)|0x4000)
		return 2
	case v < 1<<30:
		binary.BigEndian.PutUint32(b[:4], uint32(v)|0x80000000)
		return 4
	default:
		binary.BigEndian.PutUint64(b[:8], v|0xc000000000000000)
		return 8
	}
}

// EncodeVarint encodes val as a QUIC-style variable length integer (RFC 9000 §16).
func EncodeVarint(val uint64) []byte {
	switch {
	case val <= 63:
		return []byte{byte(val)}
	case val <= 16383:
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(val)|0x4000)
		return b
	case val <= 1073741823:
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(val)|0x80000000)
		return b
	default:
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, val|0xc000000000000000)
		return b
	}
}

// DecodeVarint decodes a QUIC-style variable length integer from payload.
func DecodeVarint(payload []byte) (val uint64, readLen int, err error) {
	if len(payload) == 0 {
		return 0, 0, errors.New("sein/masque: truncated varint payload")
	}

	first := payload[0]
	tag := first >> 6

	switch tag {
	case 0:
		return uint64(first & 0x3f), 1, nil
	case 1:
		if len(payload) < 2 {
			return 0, 0, errors.New("sein/masque: truncated 2-byte varint")
		}
		v := binary.BigEndian.Uint16(payload[:2]) & 0x3fff
		return uint64(v), 2, nil
	case 2:
		if len(payload) < 4 {
			return 0, 0, errors.New("sein/masque: truncated 4-byte varint")
		}
		v := binary.BigEndian.Uint32(payload[:4]) & 0x3fffffff
		return uint64(v), 4, nil
	case 3:
		if len(payload) < 8 {
			return 0, 0, errors.New("sein/masque: truncated 8-byte varint")
		}
		v := binary.BigEndian.Uint64(payload[:8]) & 0x3fffffffffffffff
		return v, 8, nil
	}

	return 0, 0, errors.New("sein/masque: invalid varint tag")
}
