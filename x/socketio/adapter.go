// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socketio

import (
	"slices"
	"sync"
)

// Adapter manages room membership and message dispatching across sockets.
type Adapter interface {
	// Add associates a socket ID with one or more rooms.
	Add(id SocketID, rooms ...Room)

	// Del removes a socket ID from one or more rooms.
	Del(id SocketID, rooms ...Room)

	// DelAll removes a socket ID from all joined rooms.
	DelAll(id SocketID)

	// Broadcast distributes a packet according to the broadcast options.
	Broadcast(pkt Packet, opts BroadcastOptions, sendFn func(id SocketID, pkt Packet))

	// Sockets returns a slice of socket IDs belonging to the specified rooms (or all sockets if rooms is empty).
	Sockets(rooms ...Room) []SocketID

	// SocketRooms returns all rooms currently joined by the specified socket ID.
	SocketRooms(id SocketID) []Room
}

// MemoryAdapter is a concurrent in-memory implementation of Adapter.
type MemoryAdapter struct {
	mu    sync.RWMutex
	rooms map[Room]map[SocketID]struct{}
	sids  map[SocketID]map[Room]struct{}
}

// NewMemoryAdapter instantiates a new thread-safe in-memory adapter.
func NewMemoryAdapter() *MemoryAdapter {
	return &MemoryAdapter{
		rooms: make(map[Room]map[SocketID]struct{}),
		sids:  make(map[SocketID]map[Room]struct{}),
	}
}

// Add associates a socket ID with one or more rooms.
func (a *MemoryAdapter) Add(id SocketID, rooms ...Room) {
	a.mu.Lock()
	defer a.mu.Unlock()

	idRooms, ok := a.sids[id]
	if !ok {
		idRooms = make(map[Room]struct{})
		a.sids[id] = idRooms
	}

	for _, room := range rooms {
		idRooms[room] = struct{}{}

		roomMap, ok := a.rooms[room]
		if !ok {
			roomMap = make(map[SocketID]struct{})
			a.rooms[room] = roomMap
		}
		roomMap[id] = struct{}{}
	}
}

// Del removes a socket ID from one or more rooms.
func (a *MemoryAdapter) Del(id SocketID, rooms ...Room) {
	a.mu.Lock()
	defer a.mu.Unlock()

	idRooms, ok := a.sids[id]
	if !ok {
		return
	}

	for _, room := range rooms {
		delete(idRooms, room)

		if roomMap, ok := a.rooms[room]; ok {
			delete(roomMap, id)
			if len(roomMap) == 0 {
				delete(a.rooms, room)
			}
		}
	}
}

// DelAll removes a socket ID from all joined rooms and releases tracked state.
func (a *MemoryAdapter) DelAll(id SocketID) {
	a.mu.Lock()
	defer a.mu.Unlock()

	idRooms, ok := a.sids[id]
	if !ok {
		return
	}

	for room := range idRooms {
		if roomMap, ok := a.rooms[room]; ok {
			delete(roomMap, id)
			if len(roomMap) == 0 {
				delete(a.rooms, room)
			}
		}
	}

	delete(a.sids, id)
}

// Broadcast distributes a packet according to the broadcast options.
func (a *MemoryAdapter) Broadcast(pkt Packet, opts BroadcastOptions, sendFn func(id SocketID, pkt Packet)) {
	a.mu.RLock()

	targets := make(map[SocketID]struct{})

	if len(opts.Rooms) > 0 {
		for _, room := range opts.Rooms {
			if roomMap, ok := a.rooms[room]; ok {
				for sid := range roomMap {
					targets[sid] = struct{}{}
				}
			}
		}
	} else {
		for sid := range a.sids {
			targets[sid] = struct{}{}
		}
	}

	if len(opts.Except) > 0 {
		for _, exc := range opts.Except {
			// Check if exc is a socket ID
			delete(targets, exc)
			// Check if exc is a room
			if roomMap, ok := a.rooms[exc]; ok {
				for sid := range roomMap {
					delete(targets, sid)
				}
			}
		}
	}

	targetList := make([]SocketID, 0, len(targets))
	for sid := range targets {
		targetList = append(targetList, sid)
	}

	a.mu.RUnlock()

	for _, sid := range targetList {
		sendFn(sid, pkt)
	}
}

// Sockets returns all socket IDs in the given rooms (or all active if rooms is empty).
func (a *MemoryAdapter) Sockets(rooms ...Room) []SocketID {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(rooms) == 0 {
		res := make([]SocketID, 0, len(a.sids))
		for sid := range a.sids {
			res = append(res, sid)
		}
		return res
	}

	seen := make(map[SocketID]struct{})
	for _, room := range rooms {
		if roomMap, ok := a.rooms[room]; ok {
			for sid := range roomMap {
				seen[sid] = struct{}{}
			}
		}
	}

	res := make([]SocketID, 0, len(seen))
	for sid := range seen {
		res = append(res, sid)
	}
	return res
}

// SocketRooms returns all rooms the socket ID is currently participating in.
func (a *MemoryAdapter) SocketRooms(id SocketID) []Room {
	a.mu.RLock()
	defer a.mu.RUnlock()

	idRooms, ok := a.sids[id]
	if !ok {
		return nil
	}

	res := make([]Room, 0, len(idRooms))
	for room := range idRooms {
		res = append(res, room)
	}
	slices.Sort(res)
	return res
}
