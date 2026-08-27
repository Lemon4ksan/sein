// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socketio

import (
	"encoding/json"
	"fmt"
)

// BroadcastOperator provides a fluent builder interface for filtering targets and broadcasting events.
type BroadcastOperator struct {
	nsp  *Namespace
	opts BroadcastOptions
}

// NewBroadcastOperator creates a new broadcast builder rooted on a namespace.
func NewBroadcastOperator(nsp *Namespace) *BroadcastOperator {
	return &BroadcastOperator{
		nsp: nsp,
		opts: BroadcastOptions{
			Rooms:  make([]Room, 0, 4),
			Except: make([]string, 0, 4),
		},
	}
}

// To targets one or more rooms for message broadcast.
func (b *BroadcastOperator) To(rooms ...Room) *BroadcastOperator {
	b.opts.Rooms = append(b.opts.Rooms, rooms...)
	return b
}

// In is an alias for To.
func (b *BroadcastOperator) In(rooms ...Room) *BroadcastOperator {
	return b.To(rooms...)
}

// Except excludes specific socket IDs or rooms from receiving the broadcast.
func (b *BroadcastOperator) Except(except ...string) *BroadcastOperator {
	b.opts.Except = append(b.opts.Except, except...)
	return b
}

// Volatile marks the packet as drop-safe if client buffers are saturated or unready.
func (b *BroadcastOperator) Volatile() *BroadcastOperator {
	b.opts.Volatile = true
	return b
}

// Emit broadcasts the event and arguments to all matching sockets.
func (b *BroadcastOperator) Emit(event string, args ...any) error {
	payload := make([]any, 1+len(args))
	payload[0] = event
	copy(payload[1:], args)

	if hasBinary(payload) {
		deconstructed, buffers := deconstructBinary(payload)
		jsonData, err := json.Marshal(deconstructed)
		if err != nil {
			return fmt.Errorf("socketio: marshal broadcast binary payload: %w", err)
		}

		pkt := Packet{
			Type:        sioBinaryEvent,
			Namespace:   b.nsp.Name(),
			Attachments: len(buffers),
			Data:        jsonData,
		}

		b.nsp.adapter.Broadcast(pkt, b.opts, func(id SocketID, p Packet) {
			if s := b.nsp.Socket(id); s != nil {
				_ = s.sendBinaryPacket(p, buffers)
			}
		})

		return nil
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("socketio: marshal broadcast payload: %w", err)
	}

	pkt := Packet{
		Type:      sioEvent,
		Namespace: b.nsp.Name(),
		Data:      jsonData,
	}

	b.nsp.adapter.Broadcast(pkt, b.opts, func(id SocketID, p Packet) {
		if s := b.nsp.Socket(id); s != nil {
			_ = s.sendPacket(p)
		}
	})

	return nil
}

// DisconnectSockets terminates matching sockets across the namespace.
func (b *BroadcastOperator) DisconnectSockets(closeUnderlying bool) {
	sids := b.nsp.adapter.Sockets(b.opts.Rooms...)
	exceptSet := make(map[string]struct{}, len(b.opts.Except))
	for _, exc := range b.opts.Except {
		exceptSet[exc] = struct{}{}
	}

	for _, sid := range sids {
		if _, ok := exceptSet[sid]; ok {
			continue
		}
		if s := b.nsp.Socket(sid); s != nil {
			s.Disconnect(closeUnderlying)
		}
	}
}
