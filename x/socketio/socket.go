// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socketio

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/async/task"
)

// Socket represents an active client connection bound to a specific Socket.IO namespace.
type Socket struct {
	id        SocketID
	nsp       *Namespace
	session   *engineSession
	handshake HandshakeData

	dataMu sync.RWMutex
	data   any

	eventsMu           sync.RWMutex
	events             map[string][]func(ack AckFunc, args []json.RawMessage)
	anyEvents          []func(event string, args []json.RawMessage)
	disconnectHandlers []func(reason string)
	errorHandlers      []func(err error)

	ackMgr *task.Manager[int64, []json.RawMessage]
	closed atomic.Bool
}

func newSocket(nsp *Namespace, session *engineSession, authPayload json.RawMessage) *Socket {
	hs := session.handshake
	if len(authPayload) > 0 {
		hs.Auth = authPayload
	}

	s := &Socket{
		id:        session.sid,
		nsp:       nsp,
		session:   session,
		handshake: hs,
		events:    make(map[string][]func(ack AckFunc, args []json.RawMessage)),
		ackMgr:    task.NewManager[int64, []json.RawMessage](0),
	}

	// Automatically join default self room
	nsp.adapter.Add(s.id, s.id)

	return s
}

// ID returns the unique socket identifier.
func (s *Socket) ID() SocketID {
	return s.id
}

// Nsp returns the namespace this socket is attached to.
func (s *Socket) Nsp() *Namespace {
	return s.nsp
}

// Handshake returns a copy of the handshake metadata.
func (s *Socket) Handshake() HandshakeData {
	return s.handshake
}

// Data returns user-defined context attached to this socket.
func (s *Socket) Data() any {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()
	return s.data
}

// SetData stores user-defined context on this socket.
func (s *Socket) SetData(val any) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()
	s.data = val
}

// On registers a listener for the specified event name.
func (s *Socket) On(event string, handler func(args []json.RawMessage)) {
	s.OnEvent(event, func(_ AckFunc, args []json.RawMessage) {
		handler(args)
	})
}

// OnEvent registers an event handler with explicit access to the acknowledgment callback.
func (s *Socket) OnEvent(event string, handler func(ack AckFunc, args []json.RawMessage)) {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()

	s.events[event] = append(s.events[event], handler)
}

// OnWithAck registers an event handler whose returned values will automatically form the acknowledgment response.
func (s *Socket) OnWithAck(event string, handler func(args []json.RawMessage) ([]any, error)) {
	s.OnEvent(event, func(ack AckFunc, args []json.RawMessage) {
		res, err := handler(args)
		if ack != nil {
			if err != nil {
				ack(map[string]string{"error": err.Error()})
				return
			}
			ack(res...)
		}
	})
}

// OnAny registers a catch-all listener for all incoming events on this socket.
func (s *Socket) OnAny(handler func(event string, args []json.RawMessage)) {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()

	s.anyEvents = append(s.anyEvents, handler)
}

// OnDisconnect registers a callback invoked when the socket disconnects.
func (s *Socket) OnDisconnect(handler func(reason string)) {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()

	s.disconnectHandlers = append(s.disconnectHandlers, handler)
}

// OnError registers a callback invoked when an error occurs on this socket.
func (s *Socket) OnError(handler func(err error)) {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()

	s.errorHandlers = append(s.errorHandlers, handler)
}

// Join associates this socket with one or more rooms.
func (s *Socket) Join(rooms ...Room) {
	s.nsp.adapter.Add(s.id, rooms...)
}

// Leave removes this socket from one or more rooms.
func (s *Socket) Leave(rooms ...Room) {
	s.nsp.adapter.Del(s.id, rooms...)
}

// Rooms returns the list of rooms this socket is currently in.
func (s *Socket) Rooms() []Room {
	return s.nsp.adapter.SocketRooms(s.id)
}

// Broadcast returns a BroadcastOperator targeting all sockets in the namespace except this one.
func (s *Socket) Broadcast() *BroadcastOperator {
	return s.nsp.Broadcast().Except(s.id)
}

// To returns a BroadcastOperator targeting specific rooms in the namespace except this socket.
func (s *Socket) To(rooms ...Room) *BroadcastOperator {
	return s.nsp.To(rooms...).Except(s.id)
}

// In is an alias for To.
func (s *Socket) In(rooms ...Room) *BroadcastOperator {
	return s.To(rooms...)
}

// Emit sends an event to this client socket.
func (s *Socket) Emit(event string, args ...any) error {
	if s.closed.Load() {
		return ErrSocketClosed
	}

	payload := make([]any, 1+len(args))
	payload[0] = event
	copy(payload[1:], args)

	if hasBinary(payload) {
		deconstructed, buffers := deconstructBinary(payload)
		jsonData, err := json.Marshal(deconstructed)
		if err != nil {
			return fmt.Errorf("socketio: marshal binary event: %w", err)
		}

		pkt := Packet{
			Type:        sioBinaryEvent,
			Namespace:   s.nsp.Name(),
			Attachments: len(buffers),
			Data:        jsonData,
		}

		return s.sendBinaryPacket(pkt, buffers)
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("socketio: marshal event: %w", err)
	}

	pkt := Packet{
		Type:      sioEvent,
		Namespace: s.nsp.Name(),
		Data:      jsonData,
	}

	return s.sendPacket(pkt)
}

// EmitWithAck sends an event and waits synchronously until an acknowledgment is returned or context expires.
func (s *Socket) EmitWithAck(ctx context.Context, event string, args ...any) ([]json.RawMessage, error) {
	if s.closed.Load() {
		return nil, ErrSocketClosed
	}

	payload := make([]any, 1+len(args))
	payload[0] = event
	copy(payload[1:], args)

	id := s.ackMgr.NextID()
	ch := make(chan []json.RawMessage, 1)

	_ = s.ackMgr.Add(id, func(_ context.Context, response []json.RawMessage, err error) {
		if err == nil {
			select {
			case ch <- response:
			default:
			}
		}
	}, task.WithTimeout[[]json.RawMessage](30*time.Second))

	var err error
	if hasBinary(payload) {
		deconstructed, buffers := deconstructBinary(payload)
		jsonData, marshalErr := json.Marshal(deconstructed)
		if marshalErr != nil {
			return nil, fmt.Errorf("socketio: marshal binary event: %w", marshalErr)
		}

		pkt := Packet{
			Type:        sioBinaryEvent,
			Namespace:   s.nsp.Name(),
			ID:          &id,
			Attachments: len(buffers),
			Data:        jsonData,
		}
		err = s.sendBinaryPacket(pkt, buffers)
	} else {
		jsonData, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return nil, fmt.Errorf("socketio: marshal event: %w", marshalErr)
		}

		pkt := Packet{
			Type:      sioEvent,
			Namespace: s.nsp.Name(),
			ID:        &id,
			Data:      jsonData,
		}
		err = s.sendPacket(pkt)
	}

	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res, nil
	}
}

// EmitVolatile sends an event only if the socket connection is healthy, silently ignoring errors.
func (s *Socket) EmitVolatile(event string, args ...any) error {
	if s.closed.Load() {
		return nil
	}
	return s.Emit(event, args...)
}

// Disconnect terminates this namespace session. If closeUnderlying is true, the transport connection is severed.
func (s *Socket) Disconnect(closeUnderlying bool) {
	if s.closed.CompareAndSwap(false, true) {
		_ = s.sendPacket(Packet{
			Type:      sioDisconnect,
			Namespace: s.nsp.Name(),
		})

		s.cleanup("server namespace disconnect")

		if closeUnderlying {
			_ = s.session.Close("server disconnect")
		}
	}
}

func (s *Socket) sendPacket(pkt Packet) error {
	if s.closed.Load() {
		return ErrSocketClosed
	}

	encoded := EncodePacket(pkt)
	return s.session.writeEIOPacket(eioMessage, encoded)
}

func (s *Socket) sendBinaryPacket(pkt Packet, buffers [][]byte) error {
	if s.closed.Load() {
		return ErrSocketClosed
	}

	encoded := EncodePacket(pkt)
	if err := s.session.writeEIOPacket(eioMessage, encoded); err != nil {
		return err
	}

	for _, buf := range buffers {
		if err := s.session.writeBinaryAttachment(buf); err != nil {
			return err
		}
	}

	return nil
}

func (s *Socket) sendAck(id int64, args ...any) {
	if s.closed.Load() {
		return
	}

	if hasBinary(args) {
		deconstructed, buffers := deconstructBinary(args)
		jsonData, err := json.Marshal(deconstructed)
		if err != nil {
			return
		}

		pkt := Packet{
			Type:        sioBinaryAck,
			Namespace:   s.nsp.Name(),
			ID:          &id,
			Attachments: len(buffers),
			Data:        jsonData,
		}
		_ = s.sendBinaryPacket(pkt, buffers)
		return
	}

	jsonData, err := json.Marshal(args)
	if err != nil {
		return
	}

	pkt := Packet{
		Type:      sioAck,
		Namespace: s.nsp.Name(),
		ID:        &id,
		Data:      jsonData,
	}
	_ = s.sendPacket(pkt)
}

func (s *Socket) dispatchEvent(pkt *Packet) {
	var rawArgs []json.RawMessage
	if err := json.Unmarshal(pkt.Data, &rawArgs); err != nil || len(rawArgs) == 0 {
		return
	}

	var eventName string
	if err := json.Unmarshal(rawArgs[0], &eventName); err != nil {
		return
	}

	args := rawArgs[1:]

	var ackFn AckFunc
	if pkt.ID != nil {
		ackID := *pkt.ID
		ackFn = func(replyArgs ...any) {
			s.sendAck(ackID, replyArgs...)
		}
	}

	s.eventsMu.RLock()
	handlers := slices.Clone(s.events[eventName])
	anyHandlers := slices.Clone(s.anyEvents)
	s.eventsMu.RUnlock()

	for _, h := range handlers {
		go h(ackFn, args)
	}

	for _, h := range anyHandlers {
		go h(eventName, args)
	}
}

func (s *Socket) resolveAck(id int64, data json.RawMessage) {
	var args []json.RawMessage
	if err := json.Unmarshal(data, &args); err != nil {
		return
	}
	s.ackMgr.Resolve(id, args, nil)
}

func (s *Socket) cleanup(reason string) {
	if s.closed.CompareAndSwap(false, true) {
		s.nsp.removeSocket(s.id)
		s.nsp.adapter.DelAll(s.id)

		s.eventsMu.RLock()
		handlers := slices.Clone(s.disconnectHandlers)
		s.eventsMu.RUnlock()

		for _, h := range handlers {
			go h(reason)
		}
	}
}
