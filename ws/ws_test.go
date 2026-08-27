// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/ws"
)

func TestWebSocket_HandshakeAndEcho(t *testing.T) {
	app := sein.New()

	sein.Handle(app, "GET", "/ws", func(req *sein.Request) (any, error) {
		conn, err := ws.Upgrade(req)
		if err != nil {
			return nil, err
		}
		defer conn.Close()

		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return nil, nil
			}

			if err := conn.WriteMessage(msgType, data); err != nil {
				return nil, err
			}
		}
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()

	go func() {
		_ = app.Serve(ln)
	}()

	// Connect TCP client
	clientConn, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	defer clientConn.Close()

	// 1. Perform WebSocket Handshake
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	clientKey := base64.StdEncoding.EncodeToString(nonce)

	reqStr := "GET /ws HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + clientKey + "\r\n\r\n"

	_, err = clientConn.Write([]byte(reqStr))
	require.NoError(t, err)

	br := bufio.NewReader(clientConn)

	// Read Status Line
	statusLine, err := br.ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, statusLine, "101 Switching Protocols")

	// Read Headers
	expectedAccept := ws.ComputeAcceptKey(clientKey)

	var hasUpgrade, hasAccept bool
	for {
		line, err := br.ReadString('\n')
		require.NoError(t, err)

		if line == "\r\n" {
			break
		}

		if line == "Upgrade: websocket\r\n" {
			hasUpgrade = true
		}

		if line == "Sec-WebSocket-Accept: "+expectedAccept+"\r\n" {
			hasAccept = true
		}
	}

	assert.True(t, hasUpgrade)
	assert.True(t, hasAccept)

	// 2. Send Masked Text Frame from client
	sendMsg := "Hello WebSocket from Client 2026!"
	payload := []byte(sendMsg)
	maskKey := [4]byte{0x12, 0x34, 0x56, 0x78}

	maskedPayload := make([]byte, len(payload))
	for i, b := range payload {
		maskedPayload[i] = b ^ maskKey[i%4]
	}

	frameHdr := []byte{
		0x81,                      // FIN + OpText
		0x80 | byte(len(payload)), // Masked bit + Length
		maskKey[0], maskKey[1], maskKey[2], maskKey[3],
	}

	_, err = clientConn.Write(frameHdr)
	require.NoError(t, err)
	_, err = clientConn.Write(maskedPayload)
	require.NoError(t, err)

	// 3. Read Server Echo (Unmasked)
	var respHdr [2]byte

	_, err = io.ReadFull(br, respHdr[:])
	require.NoError(t, err)
	assert.Equal(t, byte(0x81), respHdr[0]) // FIN + OpText
	respLen := int(respHdr[1] & 0x7F)
	assert.Equal(t, len(payload), respLen)

	respPayload := make([]byte, respLen)
	_, err = io.ReadFull(br, respPayload)
	require.NoError(t, err)
	assert.Equal(t, sendMsg, string(respPayload))

	// 4. Send Close Frame
	closeFrame := []byte{
		0x88, // FIN + OpClose
		0x82, // Masked + 2 bytes
		maskKey[0], maskKey[1], maskKey[2], maskKey[3],
		byte(ws.StatusNormalClosure>>8) ^ maskKey[0],
		byte(ws.StatusNormalClosure&0xFF) ^ maskKey[1],
	}
	_, _ = clientConn.Write(closeFrame)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, app.Shutdown(ctx))
}

func TestWebSocket_SubprotocolNegotiation(t *testing.T) {
	app := sein.New()

	sein.Handle(app, "GET", "/ws-proto", func(req *sein.Request) (any, error) {
		conn, err := ws.Upgrade(req, ws.WithSubprotocols("graphql-transport-ws", "json-rpc"))
		if err != nil {
			return nil, err
		}
		defer conn.Close()

		return nil, nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()

	go func() {
		_ = app.Serve(ln)
	}()

	clientConn, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	defer clientConn.Close()

	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	clientKey := base64.StdEncoding.EncodeToString(nonce)

	reqStr := "GET /ws-proto HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + clientKey + "\r\n" +
		"Sec-WebSocket-Protocol: mqtt, json-rpc\r\n\r\n"

	_, err = clientConn.Write([]byte(reqStr))
	require.NoError(t, err)

	br := bufio.NewReader(clientConn)
	statusLine, err := br.ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, statusLine, "101 Switching Protocols")

	var matchedProto string
	for {
		line, err := br.ReadString('\n')
		require.NoError(t, err)

		if line == "\r\n" {
			break
		}

		if len(line) > 24 && line[:24] == "Sec-WebSocket-Protocol: " {
			matchedProto = line[24 : len(line)-2]
		}
	}

	assert.Equal(t, "json-rpc", matchedProto)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, app.Shutdown(ctx))
}

func TestWebSocket_InvalidHandshake_Rejection(t *testing.T) {
	app := sein.New()

	sein.Handle(app, "POST", "/ws-invalid", func(req *sein.Request) (any, error) {
		_, err := ws.Upgrade(req)
		assert.Equal(t, ws.ErrNotWebSocket, err)
		return nil, err
	})

	sein.Handle(app, "GET", "/ws-badkey", func(req *sein.Request) (any, error) {
		_, err := ws.Upgrade(req)
		assert.Equal(t, ws.ErrMissingKey, err)
		return nil, err
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()

	go func() {
		_ = app.Serve(ln)
	}()

	// 1. Test POST method rejection (RFC 6455 §4.2.1 Item 1)
	conn1, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	_, _ = conn1.Write(
		[]byte(
			"POST /ws-invalid HTTP/1.1\r\nHost: " + addr + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n",
		),
	)
	_ = conn1.Close()

	// 2. Test Invalid Key (not 16 bytes decoded) (RFC 6455 §4.2.1 Item 5)
	conn2, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	_, _ = conn2.Write(
		[]byte(
			"GET /ws-badkey HTTP/1.1\r\nHost: " + addr + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: YWJjZA==\r\n\r\n",
		),
	)
	_ = conn2.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, app.Shutdown(ctx))
}

func TestWS_Framing_Masking_And_AcceptKey(t *testing.T) {
	// 1. RFC 6455 §4.2.2 Sec-WebSocket-Accept test vector
	acceptKey := ws.ComputeAcceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	assert.Equal(t, "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=", acceptKey)

	// 2. BuildFrameHeader lengths: <126, <=65535, >65535
	var hdr [14]byte

	// Short frame (length 50)
	n1 := ws.BuildFrameHeader(hdr[:], 0x1, 50, false, false)
	assert.Equal(t, 2, n1)
	assert.Equal(t, byte(0x81), hdr[0])
	assert.Equal(t, byte(50), hdr[1])

	// Medium frame (length 1000)
	n2 := ws.BuildFrameHeader(hdr[:], 0x2, 1000, true, true)
	assert.Equal(t, 4, n2)
	assert.Equal(t, byte(0xC2), hdr[0]) // 0x80 | 0x40 | 0x02
	assert.Equal(t, byte(0x80|126), hdr[1])

	// Large frame (length 70000)
	n3 := ws.BuildFrameHeader(hdr[:], 0x1, 70000, false, false)
	assert.Equal(t, 10, n3)
	assert.Equal(t, byte(127), hdr[1])

	// 3. Masking roundtrip
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}
	original := []byte("WebSocket zero-alloc framing and masking test!")
	payload := append([]byte(nil), original...)

	ws.ApplyMask(payload, mask)
	assert.False(t, string(payload) == string(original))

	ws.VectorApplyFastMask(payload, mask)
	assert.Equal(t, string(original), string(payload))

	// Empty mask
	ws.ApplyMask(nil, mask)
}

func BenchmarkWS_SIMDDemasking_64KB(b *testing.B) {
	payload := make([]byte, 64*1024)
	maskKey := [4]byte{0xDE, 0xAD, 0xBE, 0xEF}

	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ws.VectorApplyFastMask(payload, maskKey)
	}
}
