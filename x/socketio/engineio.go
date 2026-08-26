// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socketio

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/sein/ws"
)

type engineSession struct {
	sid        string
	conn       *ws.Conn
	server     *Server
	handshake  HandshakeData
	writeMu    sync.Mutex
	closed     atomic.Bool
	done       chan struct{}
	lastPing   atomic.Int64
	namespaces sync.Map // map[string]*Socket
	binaryBuf  *binaryReconstructor
	binMu      sync.Mutex
}

func generateSID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func newEngineSession(server *Server, conn *ws.Conn, handshake HandshakeData) *engineSession {
	sess := &engineSession{
		sid:       generateSID(),
		conn:      conn,
		server:    server,
		handshake: handshake,
		done:      make(chan struct{}),
	}
	sess.lastPing.Store(time.Now().UnixNano())
	return sess
}

func (s *engineSession) start(ctx context.Context) error {
	// Send Engine.IO Open packet
	openPayload, err := json.Marshal(map[string]any{
		"sid":          s.sid,
		"upgrades":     []string{},
		"pingInterval": s.server.config.PingInterval.Milliseconds(),
		"pingTimeout":  s.server.config.PingTimeout.Milliseconds(),
		"maxPayload":   s.server.config.MaxPayload,
	})
	if err != nil {
		return fmt.Errorf("socketio: marshal eio open: %w", err)
	}

	if err := s.writeEIOPacket(eioOpen, openPayload); err != nil {
		return fmt.Errorf("socketio: write eio open: %w", err)
	}

	go s.heartbeatCheckLoop()
	go s.readLoop()

	return nil
}

func (s *engineSession) writeEIOPacket(pType byte, payload []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if s.closed.Load() {
		return ErrSocketClosed
	}

	data := make([]byte, 1+len(payload))
	data[0] = pType
	copy(data[1:], payload)

	return s.conn.WriteMessage(ws.OpText, data)
}

func (s *engineSession) writeBinaryAttachment(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if s.closed.Load() {
		return ErrSocketClosed
	}

	return s.conn.WriteMessage(ws.OpBinary, data)
}

func (s *engineSession) heartbeatCheckLoop() {
	timeout := s.server.config.PingInterval + s.server.config.PingTimeout
	ticker := time.NewTicker(s.server.config.PingInterval / 2)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			last := time.Unix(0, s.lastPing.Load())
			if time.Since(last) > timeout {
				_ = s.Close("ping timeout")
				return
			}
		}
	}
}

func (s *engineSession) readLoop() {
	defer func() {
		_ = s.Close("transport close")
	}()

	for {
		msgType, payload, err := s.conn.ReadMessage()
		if err != nil {
			return
		}

		s.lastPing.Store(time.Now().UnixNano())

		if msgType == ws.OpBinary {
			s.handleBinaryFrame(payload)
			continue
		}

		if len(payload) == 0 {
			continue
		}

		eioType := payload[0]
		data := payload[1:]

		switch eioType {
		case eioPing:
			_ = s.writeEIOPacket(eioPong, data)
		case eioPong:
			// Heartbeat refreshed
		case eioMessage:
			if len(data) > 0 {
				s.dispatchSIOPacket(data)
			}
		case eioClose:
			return
		}
	}
}

func (s *engineSession) handleBinaryFrame(data []byte) {
	s.binMu.Lock()
	defer s.binMu.Unlock()

	if s.binaryBuf != nil && s.binaryBuf.addBuffer(data) {
		pkt, err := s.binaryBuf.reconstruct()
		s.binaryBuf = nil
		if err == nil {
			s.routeSIOPacket(pkt)
		}
	}
}

func (s *engineSession) dispatchSIOPacket(data []byte) {
	pkt, err := DecodePacket(data)
	if err != nil {
		return
	}

	if pkt.Type == sioBinaryEvent || pkt.Type == sioBinaryAck {
		s.binMu.Lock()
		s.binaryBuf = newBinaryReconstructor(pkt.Attachments, pkt)
		s.binMu.Unlock()
		return
	}

	s.routeSIOPacket(pkt)
}

func (s *engineSession) routeSIOPacket(pkt *Packet) {
	switch pkt.Type {
	case sioConnect:
		s.handleConnect(pkt)
	case sioDisconnect:
		s.handleDisconnect(pkt.Namespace)
	case sioEvent:
		s.handleEvent(pkt)
	case sioAck:
		s.handleAck(pkt)
	}
}

func (s *engineSession) handleConnect(pkt *Packet) {
	nsp := s.server.Of(pkt.Namespace)
	if nsp == nil {
		errPayload, _ := json.Marshal(map[string]string{"message": "Invalid namespace"})
		_ = s.writeEIOPacket(eioMessage, EncodePacket(Packet{
			Type:      sioConnectError,
			Namespace: pkt.Namespace,
			Data:      errPayload,
		}))
		return
	}

	// Create new socket inside the namespace
	socket := newSocket(nsp, s, pkt.Data)
	s.namespaces.Store(pkt.Namespace, socket)

	// Run namespace middlewares
	nsp.runMiddlewares(socket, func(err error) {
		if err != nil {
			s.namespaces.Delete(pkt.Namespace)
			errPayload, _ := json.Marshal(map[string]string{"message": err.Error()})
			_ = s.writeEIOPacket(eioMessage, EncodePacket(Packet{
				Type:      sioConnectError,
				Namespace: pkt.Namespace,
				Data:      errPayload,
			}))
			return
		}

		nsp.registerSocket(socket)

		// Send connect success packet
		connectResp, _ := json.Marshal(map[string]string{"sid": socket.id})
		_ = s.writeEIOPacket(eioMessage, EncodePacket(Packet{
			Type:      sioConnect,
			Namespace: pkt.Namespace,
			Data:      connectResp,
		}))

		// Trigger connection handlers
		nsp.triggerConnect(socket)
	})
}

func (s *engineSession) handleDisconnect(namespace string) {
	if val, ok := s.namespaces.LoadAndDelete(namespace); ok {
		socket := val.(*Socket)
		socket.cleanup("client namespace disconnect")
	}
}

func (s *engineSession) handleEvent(pkt *Packet) {
	if val, ok := s.namespaces.Load(pkt.Namespace); ok {
		socket := val.(*Socket)
		socket.dispatchEvent(pkt)
	}
}

func (s *engineSession) handleAck(pkt *Packet) {
	if val, ok := s.namespaces.Load(pkt.Namespace); ok {
		socket := val.(*Socket)
		if pkt.ID != nil {
			socket.resolveAck(*pkt.ID, pkt.Data)
		}
	}
}

func (s *engineSession) Close(reason string) error {
	if s.closed.CompareAndSwap(false, true) {
		close(s.done)

		// Clean up all namespace sockets
		s.namespaces.Range(func(key, value any) bool {
			socket := value.(*Socket)
			socket.cleanup(reason)
			return true
		})

		_ = s.writeEIOPacket(eioClose, nil)
		_ = s.conn.Close()
	}
	return nil
}
