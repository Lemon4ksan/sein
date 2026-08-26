// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (!amd64 && !arm64) || purego

package ws

const hasVectorWS = false

// VectorApplyFastMask applies the 4-byte WebSocket XOR mask key to payload.
func VectorApplyFastMask(payload []byte, mask [4]byte) {
	vectorApplyFastMask(payload, mask)
}

func vectorApplyFastMask(payload []byte, mask [4]byte) {
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
}

func vectorBuildFrameHeader(dst []byte, opcode byte, length int, compress bool, isClient bool) int {
	b0 := byte(0x80 | (opcode & 0x0F))
	if compress {
		b0 |= 0x40
	}
	dst[0] = b0

	maskBit := byte(0)
	if isClient {
		maskBit = 0x80
	}

	if length < 126 {
		dst[1] = maskBit | byte(length)
		return 2
	} else if length <= 0xFFFF {
		dst[1] = maskBit | 126
		dst[2] = byte(length >> 8)
		dst[3] = byte(length)
		return 4
	} else {
		dst[1] = maskBit | 127
		dst[2] = byte(length >> 56)
		dst[3] = byte(length >> 48)
		dst[4] = byte(length >> 40)
		dst[5] = byte(length >> 32)
		dst[6] = byte(length >> 24)
		dst[7] = byte(length >> 16)
		dst[8] = byte(length >> 8)
		dst[9] = byte(length)
		return 10
	}
}
