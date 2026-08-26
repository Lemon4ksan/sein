// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package socketio provides a high-throughput, RFC-compliant Socket.IO v5 and Engine.IO v4 server for the sein framework.
//
// It is engineered for low-latency realtime communication over native WebSockets,
// featuring hierarchical namespace multiplexing, room broadcasting with custom adapter backends,
// bidirectional event acknowledgments, connection middleware pipelines, and zero-allocation binary attachments.
//
// # Basic Usage
//
//	sio := socketio.NewServer(socketio.Config{
//		PingInterval: 25 * time.Second,
//		PingTimeout:  20 * time.Second,
//	})
//
//	sio.OnConnect(func(socket *socketio.Socket) {
//		log.Printf("client connected: %s", socket.ID())
//
//		socket.On("chat:message", func(args []json.RawMessage) {
//			socket.Broadcast().Emit("chat:message", args[0])
//		})
//
//		socket.OnDisconnect(func(reason string) {
//			log.Printf("client disconnected: %s (%s)", socket.ID(), reason)
//		})
//	})
//
//	// Mount on sein server
//	server.Mount("/socket.io", sio)
package socketio
