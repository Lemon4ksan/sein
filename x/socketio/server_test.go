// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socketio

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"github.com/lemon4ksan/sein"
)

func TestMemoryAdapter(t *testing.T) {
	t.Parallel()

	adapter := NewMemoryAdapter()

	// Initial state
	assert.Empty(t, adapter.Sockets())
	assert.Nil(t, adapter.SocketRooms("s1"))

	// Add sockets to rooms
	adapter.Add("s1", "roomA", "roomB")
	adapter.Add("s2", "roomB", "roomC")
	adapter.Add("s3", "roomC")

	assert.Equal(t, []Room{"roomA", "roomB"}, adapter.SocketRooms("s1"))
	assert.Equal(t, []Room{"roomB", "roomC"}, adapter.SocketRooms("s2"))
	assert.Equal(t, []Room{"roomC"}, adapter.SocketRooms("s3"))

	// Query room sockets
	roomBSockets := adapter.Sockets("roomB")
	assert.Len(t, roomBSockets, 2)
	assert.Contains(t, roomBSockets, "s1")
	assert.Contains(t, roomBSockets, "s2")

	// Broadcast targeting roomB except s1
	var delivered []SocketID
	pkt := Packet{Type: sioEvent, Namespace: "/", Data: json.RawMessage(`["msg"]`)}
	adapter.Broadcast(pkt, BroadcastOptions{
		Rooms:  []Room{"roomB"},
		Except: []string{"s1"},
	}, func(id SocketID, p Packet) {
		delivered = append(delivered, id)
	})

	require.Len(t, delivered, 1)
	assert.Equal(t, "s2", delivered[0])

	// Remove from room
	adapter.Del("s1", "roomA")
	assert.Equal(t, []Room{"roomB"}, adapter.SocketRooms("s1"))

	// DelAll
	adapter.DelAll("s2")
	assert.Nil(t, adapter.SocketRooms("s2"))
	assert.Len(t, adapter.Sockets("roomB"), 1)
}

func TestServerLifecycleAndNamespaces(t *testing.T) {
	t.Parallel()

	server := NewServer(
		WithPingInterval(50*time.Millisecond),
		WithPingTimeout(50*time.Millisecond),
		WithConnectTimeout(5*time.Second),
	)
	t.Cleanup(func() { _ = server.Close() })

	rootNsp := server.Of("/")
	require.NotNil(t, rootNsp)
	assert.Equal(t, "/", rootNsp.Name())

	chatNsp := server.Of("/chat")
	require.NotNil(t, chatNsp)
	assert.Equal(t, "/chat", chatNsp.Name())

	// Same namespace returns cached instance
	assert.Same(t, chatNsp, server.Of("/chat"))
	assert.Same(t, chatNsp, server.Of("chat"))

	// Test middleware registration
	var mwCalled atomic.Bool
	chatNsp.Use(func(s *Socket, next func(err error)) {
		mwCalled.Store(true)
		next(nil)
	})

	// Test socket creation and connection lifecycle
	sess := &engineSession{
		sid:       "test-session-1",
		server:    server,
		handshake: HandshakeData{RemoteAddr: "127.0.0.1:12345"},
		done:      make(chan struct{}),
	}
	socket := newSocket(chatNsp, sess, nil)
	assert.Equal(t, "test-session-1", socket.ID())
	assert.Equal(t, "/chat", socket.Nsp().Name())

	var connectedSocket atomic.Pointer[Socket]
	chatNsp.OnConnect(func(s *Socket) {
		connectedSocket.Store(s)
	})

	chatNsp.runMiddlewares(socket, func(err error) {
		require.NoError(t, err)
		chatNsp.registerSocket(socket)
		chatNsp.triggerConnect(socket)
	})

	assert.True(t, mwCalled.Load())
	assert.Eventually(t, func() bool {
		return connectedSocket.Load() != nil
	}, 1*time.Second, 10*time.Millisecond)

	assert.Equal(t, socket.ID(), connectedSocket.Load().ID())
	assert.Len(t, chatNsp.Sockets(), 1)
	assert.Same(t, socket, chatNsp.Socket(socket.ID()))

	// Room operations
	socket.Join("room-1", "room-2")
	rooms := socket.Rooms()
	assert.Contains(t, rooms, "room-1")
	assert.Contains(t, rooms, "room-2")
	assert.Contains(t, rooms, socket.ID()) // Self room

	socket.Leave("room-1")
	roomsAfterLeave := socket.Rooms()
	assert.NotContains(t, roomsAfterLeave, "room-1")
	assert.Contains(t, roomsAfterLeave, "room-2")

	// Disconnect socket
	var disconnected atomic.Bool
	socket.OnDisconnect(func(reason string) {
		disconnected.Store(true)
	})

	socket.cleanup("client left")
	assert.Eventually(t, func() bool {
		return disconnected.Load()
	}, 1*time.Second, 10*time.Millisecond)
	assert.Empty(t, chatNsp.Sockets())
	assert.Nil(t, chatNsp.Socket(socket.ID()))
}

func TestServerMiddlewareRejection(t *testing.T) {
	t.Parallel()

	server := NewServer()
	t.Cleanup(func() { _ = server.Close() })

	adminNsp := server.Of("/admin")
	adminNsp.Use(func(s *Socket, next func(err error)) {
		var auth struct {
			Token string `json:"token"`
		}
		_ = json.Unmarshal(s.Handshake().Auth, &auth)
		if auth.Token != "supersecret" {
			next(errors.New("unauthorized token"))
			return
		}
		next(nil)
	})

	sess := &engineSession{
		sid:       "sess-unauth",
		server:    server,
		handshake: HandshakeData{},
		done:      make(chan struct{}),
	}

	badSocket := newSocket(adminNsp, sess, json.RawMessage(`{"token":"wrong"}`))

	var errReceived error
	adminNsp.runMiddlewares(badSocket, func(err error) {
		errReceived = err
	})

	require.Error(t, errReceived)
	assert.Equal(t, "unauthorized token", errReceived.Error())

	goodSocket := newSocket(adminNsp, sess, json.RawMessage(`{"token":"supersecret"}`))
	var goodErr error
	adminNsp.runMiddlewares(goodSocket, func(err error) {
		goodErr = err
	})
	require.NoError(t, goodErr)
}

func TestSocketEventsAndAcks(t *testing.T) {
	t.Parallel()

	server := NewServer()
	t.Cleanup(func() { _ = server.Close() })

	nsp := server.Of("/")
	sess := &engineSession{
		sid:       "sess-ack",
		server:    server,
		handshake: HandshakeData{},
		done:      make(chan struct{}),
	}
	socket := newSocket(nsp, sess, nil)
	nsp.registerSocket(socket)

	// Test regular event dispatch
	var receivedArg string
	var eventWg sync.WaitGroup
	eventWg.Add(1)

	socket.On("chat", func(args []json.RawMessage) {
		defer eventWg.Done()
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &receivedArg)
		}
	})

	socket.dispatchEvent(&Packet{
		Type:      sioEvent,
		Namespace: "/",
		Data:      json.RawMessage(`["chat","hello world"]`),
	})

	eventWg.Wait()
	assert.Equal(t, "hello world", receivedArg)

	// Test OnWithAck handler
	var ackHandled sync.WaitGroup
	ackHandled.Add(1)

	socket.OnWithAck("compute", func(args []json.RawMessage) ([]any, error) {
		defer ackHandled.Done()
		var n int
		_ = json.Unmarshal(args[0], &n)
		return []any{n * 2, true}, nil
	})

	ackID := int64(10)
	socket.dispatchEvent(&Packet{
		Type:      sioEvent,
		Namespace: "/",
		ID:        &ackID,
		Data:      json.RawMessage(`["compute",21]`),
	})

	ackHandled.Wait()

	// Test Catch-all OnAny
	var anyEvent string
	var anyWg sync.WaitGroup
	anyWg.Add(1)

	socket.OnAny(func(event string, args []json.RawMessage) {
		defer anyWg.Done()
		anyEvent = event
	})

	socket.dispatchEvent(&Packet{
		Type:      sioEvent,
		Namespace: "/",
		Data:      json.RawMessage(`["random_action",123]`),
	})

	anyWg.Wait()
	assert.Equal(t, "random_action", anyEvent)
}

func TestServerOptionsAndMount(t *testing.T) {
	t.Parallel()

	customAdapter := NewMemoryAdapter()
	originChecked := false

	sio := NewServer(
		WithPingInterval(10*time.Second),
		WithPingTimeout(5*time.Second),
		WithMaxPayload(2*1024*1024),
		WithCheckOrigin(func(req *sein.Request) bool {
			originChecked = true
			return true
		}),
		WithAdapter(customAdapter),
	)
	t.Cleanup(func() { _ = sio.Close() })

	assert.Equal(t, 10*time.Second, sio.config.PingInterval)
	assert.Equal(t, 5*time.Second, sio.config.PingTimeout)
	assert.Equal(t, int64(2*1024*1024), sio.config.MaxPayload)
	assert.Same(t, customAdapter, sio.config.Adapter)
	assert.True(t, sio.config.CheckOrigin(nil))
	assert.True(t, originChecked)

	app := sein.New()
	app.Mount("/socket.io", sio)

	// Verify Mount didn't panic and handler exists
	require.NotNil(t, sio.Handler())
}

func TestTypedBindings(t *testing.T) {
	t.Parallel()

	server := NewServer()
	t.Cleanup(func() { _ = server.Close() })

	nsp := server.Of("/")
	sess := &engineSession{
		sid:       "sess-typed",
		server:    server,
		handshake: HandshakeData{},
		done:      make(chan struct{}),
	}
	socket := newSocket(nsp, sess, nil)
	nsp.registerSocket(socket)

	type UserMsg struct {
		User string `json:"user"`
		Age  int    `json:"age"`
	}

	var received UserMsg
	var wg sync.WaitGroup
	wg.Add(1)

	Bind[UserMsg](socket, "user:profile", func(msg UserMsg) {
		defer wg.Done()
		received = msg
	})

	socket.dispatchEvent(&Packet{
		Type:      sioEvent,
		Namespace: "/",
		Data:      json.RawMessage(`["user:profile",{"user":"Alice","age":28}]`),
	})

	wg.Wait()
	assert.Equal(t, "Alice", received.User)
	assert.Equal(t, 28, received.Age)

	type AddReq struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type AddRes struct {
		Result int `json:"result"`
	}

	var ackWg sync.WaitGroup
	ackWg.Add(1)

	BindWithAck[AddReq, AddRes](socket, "math:add", func(req AddReq) (AddRes, error) {
		defer ackWg.Done()
		return AddRes{Result: req.A + req.B}, nil
	})

	ackID := int64(99)
	socket.dispatchEvent(&Packet{
		Type:      sioEvent,
		Namespace: "/",
		ID:        &ackID,
		Data:      json.RawMessage(`["math:add",{"a":15,"b":25}]`),
	})

	ackWg.Wait()
}

