// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socketio

import (
	"encoding/json"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
)

func int64Ptr(i int64) *int64 {
	return &i
}

func TestPacketEncodeDecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		packet   Packet
		expected string
	}{
		{
			name:     "connect default namespace",
			packet:   Packet{Type: sioConnect, Namespace: "/"},
			expected: "0",
		},
		{
			name:     "connect with auth",
			packet:   Packet{Type: sioConnect, Namespace: "/", Data: json.RawMessage(`{"token":"secret"}`)},
			expected: `0{"token":"secret"}`,
		},
		{
			name:     "connect custom namespace",
			packet:   Packet{Type: sioConnect, Namespace: "/chat", Data: json.RawMessage(`{"token":"secret"}`)},
			expected: `0/chat,{"token":"secret"}`,
		},
		{
			name:     "disconnect default namespace",
			packet:   Packet{Type: sioDisconnect, Namespace: "/"},
			expected: "1",
		},
		{
			name:     "disconnect custom namespace",
			packet:   Packet{Type: sioDisconnect, Namespace: "/chat"},
			expected: "1/chat,",
		},
		{
			name:     "event default namespace",
			packet:   Packet{Type: sioEvent, Namespace: "/", Data: json.RawMessage(`["message","hello"]`)},
			expected: `2["message","hello"]`,
		},
		{
			name:     "event with ack id",
			packet:   Packet{Type: sioEvent, Namespace: "/", ID: int64Ptr(42), Data: json.RawMessage(`["ping"]`)},
			expected: `242["ping"]`,
		},
		{
			name:     "event custom namespace with ack id",
			packet:   Packet{Type: sioEvent, Namespace: "/admin", ID: int64Ptr(100), Data: json.RawMessage(`["kick","user1"]`)},
			expected: `2/admin,100["kick","user1"]`,
		},
		{
			name:     "ack default namespace",
			packet:   Packet{Type: sioAck, Namespace: "/", ID: int64Ptr(42), Data: json.RawMessage(`["pong",true]`)},
			expected: `342["pong",true]`,
		},
		{
			name:     "connect error",
			packet:   Packet{Type: sioConnectError, Namespace: "/", Data: json.RawMessage(`{"message":"unauthorized"}`)},
			expected: `4{"message":"unauthorized"}`,
		},
		{
			name: "binary event 2 attachments",
			packet: Packet{
				Type:        sioBinaryEvent,
				Namespace:   "/",
				Attachments: 2,
				Data:        json.RawMessage(`["upload",{"_placeholder":true,"num":0},{"_placeholder":true,"num":1}]`),
			},
			expected: `52-["upload",{"_placeholder":true,"num":0},{"_placeholder":true,"num":1}]`,
		},
		{
			name: "binary ack custom namespace",
			packet: Packet{
				Type:        sioBinaryAck,
				Namespace:   "/chat",
				Attachments: 1,
				ID:          int64Ptr(5),
				Data:        json.RawMessage(`[{"_placeholder":true,"num":0}]`),
			},
			expected: `61-/chat,5[{"_placeholder":true,"num":0}]`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			encoded := EncodePacket(tc.packet)
			assert.Equal(t, tc.expected, string(encoded))

			decoded, err := DecodePacket(encoded)
			require.NoError(t, err)

			assert.Equal(t, tc.packet.Type, decoded.Type)
			assert.Equal(t, tc.packet.Namespace, decoded.Namespace)
			if tc.packet.ID != nil {
				require.NotNil(t, decoded.ID)
				assert.Equal(t, *tc.packet.ID, *decoded.ID)
			}
			assert.Equal(t, tc.packet.Attachments, decoded.Attachments)
			if len(tc.packet.Data) > 0 {
				assert.Equal(t, string(tc.packet.Data), string(decoded.Data))
			}
		})
	}
}

func TestBinaryDeconstructReconstruct(t *testing.T) {
	t.Parallel()

	rawBinary1 := []byte("hello-binary-1")
	rawBinary2 := []byte{0x00, 0xFF, 0xFE, 0x42}

	input := []any{
		"event:data",
		rawBinary1,
		[]any{
			rawBinary2,
			"plain-text",
		},
	}

	assert.True(t, hasBinary(input))

	deconstructed, buffers := deconstructBinary(input)
	require.Len(t, buffers, 2)
	assert.Equal(t, rawBinary1, buffers[0])
	assert.Equal(t, rawBinary2, buffers[1])

	encodedJSON, err := json.Marshal(deconstructed)
	require.NoError(t, err)

	pkt := &Packet{
		Type:        sioBinaryEvent,
		Namespace:   "/",
		Attachments: len(buffers),
		Data:        encodedJSON,
	}

	reconstructor := newBinaryReconstructor(pkt.Attachments, pkt)
	assert.False(t, reconstructor.addBuffer(buffers[0]))
	assert.True(t, reconstructor.addBuffer(buffers[1]))

	reconstructedPkt, err := reconstructor.reconstruct()
	require.NoError(t, err)
	assert.Equal(t, sioEvent, reconstructedPkt.Type)

	var output []any
	err = json.Unmarshal(reconstructedPkt.Data, &output)
	require.NoError(t, err)

	assert.Equal(t, "event:data", output[0])
}
