// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"sync"

	"github.com/lemon4ksan/foundation/generic"
)

// Hub is a thread-safe declarative WebSocket topic/room subscription manager with Pub/Sub support.
type Hub struct {
	mu     sync.RWMutex
	topics map[string]generic.Set[*Conn]
	conns  map[*Conn]generic.Set[string]
}

// NewHub instantiates an empty, initialized Pub/Sub [Hub].
func NewHub() *Hub {
	return &Hub{
		topics: make(map[string]generic.Set[*Conn]),
		conns:  make(map[*Conn]generic.Set[string]),
	}
}

// Subscribe adds a connection to the specified topic/room.
func (h *Hub) Subscribe(topic string, conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.topics[topic] == nil {
		h.topics[topic] = generic.NewSet[*Conn]()
	}
	h.topics[topic].Add(conn)

	if h.conns[conn] == nil {
		h.conns[conn] = generic.NewSet[string]()
	}
	h.conns[conn].Add(topic)
}

// Unsubscribe removes a connection from the specified topic/room.
func (h *Hub) Unsubscribe(topic string, conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if t, ok := h.topics[topic]; ok {
		delete(t, conn)
		if len(t) == 0 {
			delete(h.topics, topic)
		}
	}

	if c, ok := h.conns[conn]; ok {
		delete(c, topic)
		if len(c) == 0 {
			delete(h.conns, conn)
		}
	}
}

// UnsubscribeAll removes a connection from all subscribed topics upon disconnection.
func (h *Hub) UnsubscribeAll(conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if userTopics, ok := h.conns[conn]; ok {
		for topic := range userTopics {
			if t, ok := h.topics[topic]; ok {
				delete(t, conn)
				if len(t) == 0 {
					delete(h.topics, topic)
				}
			}
		}
		delete(h.conns, conn)
	}
}

// Publish broadcasts raw binary/text bytes to all connections subscribed to the topic using zero-copy pre-framing.
func (h *Hub) Publish(topic string, messageType int, payload []byte) error {
	frame := PrecompileFrame(messageType, payload)
	return h.PublishPrecompiled(topic, frame)
}

// PublishPrecompiled broadcasts a pre-assembled wire frame to all subscribers of topic with zero allocations.
func (h *Hub) PublishPrecompiled(topic string, frame *PrecompiledFrame) error {
	h.mu.RLock()
	connsMap, ok := h.topics[topic]
	if !ok || len(connsMap) == 0 {
		h.mu.RUnlock()
		return nil
	}

	conns := generic.Keys(connsMap)
	h.mu.RUnlock()

	var firstErr error
	for _, c := range conns {
		if err := c.WriteRawFrame(frame); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// PublishText broadcasts a UTF-8 text message to all connections subscribed to topic.
func (h *Hub) PublishText(topic string, message string) error {
	return h.Publish(topic, OpText, []byte(message))
}

// PublishJSON serializes v to JSON once and broadcasts to all subscribers of topic.
func (h *Hub) PublishJSON(topic string, v any) error {
	frame, err := PrecompileJSON(v)
	if err != nil {
		return err
	}
	return h.PublishPrecompiled(topic, frame)
}

// Broadcast broadcasts a message to ALL connected clients across all topics in the hub using zero-copy pre-framing.
func (h *Hub) Broadcast(messageType int, payload []byte) error {
	frame := PrecompileFrame(messageType, payload)
	return h.BroadcastPrecompiled(frame)
}

// BroadcastPrecompiled broadcasts a pre-assembled wire frame to ALL connected clients with zero allocations.
func (h *Hub) BroadcastPrecompiled(frame *PrecompiledFrame) error {
	h.mu.RLock()
	conns := generic.Keys(h.conns)
	h.mu.RUnlock()

	var firstErr error
	for _, c := range conns {
		if err := c.WriteRawFrame(frame); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// BroadcastJSON serializes v to JSON once and broadcasts to ALL connected clients in the hub.
func (h *Hub) BroadcastJSON(v any) error {
	frame, err := PrecompileJSON(v)
	if err != nil {
		return err
	}
	return h.BroadcastPrecompiled(frame)
}

// SubscribersCount returns the number of active subscribers to a topic.
func (h *Hub) SubscribersCount(topic string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.topics[topic])
}

// TopicsCount returns the total number of active topics with at least one subscriber.
func (h *Hub) TopicsCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.topics)
}

// ConnectionsCount returns the total number of distinct active connections in the hub.
func (h *Hub) ConnectionsCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}
