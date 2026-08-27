// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socketio_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/x/socketio"
)

type testClient struct {
	conn net.Conn
	br   *bufio.Reader
}

func dialTestClient(t *testing.T, addr, path string) *testClient {
	t.Helper()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	clientKey := base64.StdEncoding.EncodeToString(nonce)

	reqStr := fmt.Sprintf(
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: %s\r\n\r\n",
		path, addr, clientKey,
	)

	_, err = conn.Write([]byte(reqStr))
	require.NoError(t, err)

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	require.NoError(t, err)
	require.Contains(t, statusLine, "101 Switching Protocols")

	for {
		line, err := br.ReadString('\n')
		require.NoError(t, err)
		if line == "\r\n" {
			break
		}
	}

	return &testClient{
		conn: conn,
		br:   br,
	}
}

func (c *testClient) Close() {
	_ = c.conn.Close()
}

func (c *testClient) writeText(msg string) error {
	payload := []byte(msg)
	maskKey := [4]byte{0x11, 0x22, 0x33, 0x44}

	maskedPayload := make([]byte, len(payload))
	for i, b := range payload {
		maskedPayload[i] = b ^ maskKey[i%4]
	}

	frameHdr := []byte{
		0x81,
		0x80 | byte(len(payload)),
		maskKey[0], maskKey[1], maskKey[2], maskKey[3],
	}

	if _, err := c.conn.Write(frameHdr); err != nil {
		return err
	}
	_, err := c.conn.Write(maskedPayload)
	return err
}

func (c *testClient) writeBinary(data []byte) error {
	maskKey := [4]byte{0x55, 0x66, 0x77, 0x88}

	maskedPayload := make([]byte, len(data))
	for i, b := range data {
		maskedPayload[i] = b ^ maskKey[i%4]
	}

	frameHdr := []byte{
		0x82,
		0x80 | byte(len(data)),
		maskKey[0], maskKey[1], maskKey[2], maskKey[3],
	}

	if _, err := c.conn.Write(frameHdr); err != nil {
		return err
	}
	_, err := c.conn.Write(maskedPayload)
	return err
}

func (c *testClient) readMessage() (byte, []byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(c.br, hdr[:]); err != nil {
		return 0, nil, err
	}

	opcode := hdr[0] & 0x0F
	payloadLen := int(hdr[1] & 0x7F)

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		return 0, nil, err
	}

	return opcode, payload, nil
}

func TestE2E_FullSocketIOScenario(t *testing.T) {
	sio := socketio.NewServer(
		socketio.WithPingInterval(5*time.Second),
		socketio.WithPingTimeout(5*time.Second),
	)
	t.Cleanup(func() { _ = sio.Close() })

	var serverConnected atomic.Bool
	var receivedMsg atomic.Pointer[string]
	var serverSocket *socketio.Socket

	sio.OnConnect(func(socket *socketio.Socket) {
		serverConnected.Store(true)
		serverSocket = socket

		socket.On("chat:message", func(args []json.RawMessage) {
			var text string
			if len(args) > 0 {
				_ = json.Unmarshal(args[0], &text)
			}
			receivedMsg.Store(&text)
			_ = socket.Emit("chat:reply", "echo: "+text)
		})

		socket.OnWithAck("math:double", func(args []json.RawMessage) ([]any, error) {
			var n int
			if len(args) > 0 {
				_ = json.Unmarshal(args[0], &n)
			}
			return []any{n * 2}, nil
		})
	})

	app := sein.New()
	app.Mount("/socket.io", sio)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	// Connect Client
	client := dialTestClient(t, addr, "/socket.io/?EIO=4&transport=websocket")
	defer client.Close()

	// 1. Read Engine.IO Open packet
	op, data, err := client.readMessage()
	require.NoError(t, err)
	assert.Equal(t, byte(0x01), op) // Text Frame
	assert.Equal(t, byte('0'), data[0])

	var eioOpen struct {
		SID          string `json:"sid"`
		PingInterval int    `json:"pingInterval"`
		PingTimeout  int    `json:"pingTimeout"`
	}
	require.NoError(t, json.Unmarshal(data[1:], &eioOpen))
	assert.NotEmpty(t, eioOpen.SID)

	// 2. Client sends SIO Connect (40)
	require.NoError(t, client.writeText("40"))

	// Read SIO Connect response (40{"sid":"..."})
	op, data, err = client.readMessage()
	require.NoError(t, err)
	assert.Equal(t, byte(0x01), op)
	assert.Equal(t, "40", string(data[:2]))

	assert.Eventually(t, func() bool {
		return serverConnected.Load()
	}, 1*time.Second, 10*time.Millisecond)

	// 3. Client emits chat:message event (42["chat:message","Hello Sein!"])
	require.NoError(t, client.writeText(`42["chat:message","Hello Sein!"]`))

	// Read Server Reply (42["chat:reply","echo: Hello Sein!"])
	op, data, err = client.readMessage()
	require.NoError(t, err)
	assert.Equal(t, byte(0x01), op)
	assert.Equal(t, `42["chat:reply","echo: Hello Sein!"]`, string(data))
	assert.Equal(t, "Hello Sein!", *receivedMsg.Load())

	// 4. Client emits event with Ack (42100["math:double",21])
	require.NoError(t, client.writeText(`42100["math:double",21]`))

	// Read Ack Response (43100[42])
	op, data, err = client.readMessage()
	require.NoError(t, err)
	assert.Equal(t, byte(0x01), op)
	assert.Equal(t, `43100[42]`, string(data))

	// 5. Engine.IO Ping-Pong Heartbeat (2probe -> 3probe)
	require.NoError(t, client.writeText("2probe"))
	op, data, err = client.readMessage()
	require.NoError(t, err)
	assert.Equal(t, byte(0x01), op)
	assert.Equal(t, "3probe", string(data))

	// 6. Binary Emission from Server
	require.NotNil(t, serverSocket)
	binaryData := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	err = serverSocket.Emit("binary:packet", binaryData)
	require.NoError(t, err)

	// Read Binary Event Frame Header (451-["binary:packet",{"_placeholder":true,"num":0}])
	op, data, err = client.readMessage()
	require.NoError(t, err)
	assert.Equal(t, byte(0x01), op)
	assert.Equal(t, `451-["binary:packet",{"_placeholder":true,"num":0}]`, string(data))

	// Read Binary Attachment Frame (Raw Binary opcode 0x02)
	op, data, err = client.readMessage()
	require.NoError(t, err)
	assert.Equal(t, byte(0x02), op) // Binary frame
	assert.Equal(t, binaryData, data)

	// Send binary frame from client
	require.NoError(t, client.writeBinary(binaryData))

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = app.Shutdown(ctx)
}

func TestE2E_MultiClientRoomsAndNamespaces(t *testing.T) {
	sio := socketio.NewServer(
		socketio.WithPingInterval(5*time.Second),
		socketio.WithPingTimeout(5*time.Second),
	)
	t.Cleanup(func() { _ = sio.Close() })

	chatNsp := sio.Of("/chat")
	chatNsp.OnConnect(func(s *socketio.Socket) {
		s.On("join", func(args []json.RawMessage) {
			var room string
			if len(args) > 0 {
				_ = json.Unmarshal(args[0], &room)
			}
			s.Join(room)
		})

		s.On("broadcast", func(args []json.RawMessage) {
			var text string
			if len(args) > 0 {
				_ = json.Unmarshal(args[0], &text)
			}
			s.To("lounge").Emit("news", text)
		})
	})

	app := sein.New()
	app.Mount("/socket.io", sio)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	// Connect Client 1
	c1 := dialTestClient(t, addr, "/socket.io/?EIO=4&transport=websocket")
	defer c1.Close()
	_, _, err = c1.readMessage() // Open
	require.NoError(t, err)

	// Connect Client 2
	c2 := dialTestClient(t, addr, "/socket.io/?EIO=4&transport=websocket")
	defer c2.Close()
	_, _, err = c2.readMessage() // Open
	require.NoError(t, err)

	// Both connect to /chat namespace
	require.NoError(t, c1.writeText("40/chat,"))
	_, d1, err := c1.readMessage()
	require.NoError(t, err)
	assert.Contains(t, string(d1), "40/chat,")

	require.NoError(t, c2.writeText("40/chat,"))
	_, d2, err := c2.readMessage()
	require.NoError(t, err)
	assert.Contains(t, string(d2), "40/chat,")

	// c2 joins lounge
	require.NoError(t, c2.writeText(`42/chat,["join","lounge"]`))
	time.Sleep(50 * time.Millisecond)

	// c1 sends broadcast to lounge
	require.NoError(t, c1.writeText(`42/chat,["broadcast","Welcome everyone!"]`))

	// c2 receives news
	op, data, err := c2.readMessage()
	require.NoError(t, err)
	assert.Equal(t, byte(0x01), op)
	assert.Equal(t, `42/chat,["news","Welcome everyone!"]`, string(data))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = app.Shutdown(ctx)
}
