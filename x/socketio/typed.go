// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package socketio

import (
	"context"
	"encoding/json"
	"fmt"
)

// Bind registers a strongly-typed event handler that automatically deserializes incoming JSON argument into T.
func Bind[T any](s *Socket, event string, handler func(payload T)) {
	s.On(event, func(args []json.RawMessage) {
		if len(args) == 0 {
			var zero T
			handler(zero)
			return
		}

		var payload T
		if err := json.Unmarshal(args[0], &payload); err == nil {
			handler(payload)
		}
	})
}

// BindWithAck registers a strongly-typed event handler that automatically deserializes request DTO
// and serializes the response DTO into the acknowledgment packet.
func BindWithAck[Req, Res any](s *Socket, event string, handler func(req Req) (Res, error)) {
	s.OnWithAck(event, func(args []json.RawMessage) ([]any, error) {
		var req Req
		if len(args) > 0 {
			if err := json.Unmarshal(args[0], &req); err != nil {
				return nil, fmt.Errorf("socketio: unmarshal request dto: %w", err)
			}
		}

		res, err := handler(req)
		if err != nil {
			return nil, err
		}

		return []any{res}, nil
	})
}

// EmitTyped emits an event with strongly-typed data payload.
func EmitTyped[T any](s *Socket, event string, data T) error {
	return s.Emit(event, data)
}

// EmitTypedWithAck emits an event with strongly-typed request payload and decodes acknowledgment response into Res.
func EmitTypedWithAck[Req, Res any](ctx context.Context, s *Socket, event string, req Req) (Res, error) {
	var zero Res
	rawArgs, err := s.EmitWithAck(ctx, event, req)
	if err != nil {
		return zero, err
	}

	if len(rawArgs) == 0 {
		return zero, nil
	}

	var res Res
	if err := json.Unmarshal(rawArgs[0], &res); err != nil {
		return zero, fmt.Errorf("socketio: unmarshal ack response dto: %w", err)
	}

	return res, nil
}
