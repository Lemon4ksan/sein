// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpc

import (
	"errors"
	"fmt"

	"github.com/lemon4ksan/foundation/codec/json"
	"google.golang.org/protobuf/proto"
)

// Codec defines the interface gRPC uses to encode and decode messages.
type Codec interface {
	// Marshal returns the wire format of v.
	Marshal(v any) ([]byte, error)

	// Unmarshal parses the wire format into v.
	Unmarshal(data []byte, v any) error

	// Name returns the name of the Codec implementation.
	Name() string
}

type vtMarshaler interface {
	MarshalVT() ([]byte, error)
}

type vtUnmarshaler interface {
	UnmarshalVT([]byte) error
}

type legacyMarshaler interface {
	Marshal() ([]byte, error)
}

type legacyUnmarshaler interface {
	Unmarshal([]byte) error
}

// ProtoCodec is the default Codec implementation for protobuf messages.
type ProtoCodec struct{}

// Name returns proto.
func (ProtoCodec) Name() string {
	return "proto"
}

// Marshal encodes v into protobuf bytes.
func (ProtoCodec) Marshal(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}

	if vt, ok := v.(vtMarshaler); ok {
		return vt.MarshalVT()
	}

	if lm, ok := v.(legacyMarshaler); ok {
		return lm.Marshal()
	}

	if pm, ok := v.(proto.Message); ok {
		return proto.Marshal(pm)
	}

	if b, ok := v.([]byte); ok {
		return b, nil
	}

	// JSON fallback for generic types
	return json.Marshal(v)
}

// Unmarshal decodes protobuf bytes into v.
func (ProtoCodec) Unmarshal(data []byte, v any) error {
	if v == nil {
		return errors.New("cannot unmarshal into nil")
	}

	if vt, ok := v.(vtUnmarshaler); ok {
		return vt.UnmarshalVT(data)
	}

	if lm, ok := v.(legacyUnmarshaler); ok {
		return lm.Unmarshal(data)
	}

	if pm, ok := v.(proto.Message); ok {
		return proto.Unmarshal(data, pm)
	}

	if bp, ok := v.(*[]byte); ok {
		*bp = append((*bp)[:0], data...)
		return nil
	}

	// JSON fallback
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("proto unmarshal fallback failed: %w", err)
	}

	return nil
}

// JSONCodec encodes/decodes messages as JSON.
type JSONCodec struct{}

// Name returns json.
func (JSONCodec) Name() string {
	return "json"
}

// Marshal encodes v as JSON.
func (JSONCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal decodes JSON into v.
func (JSONCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

var registeredCodecs = map[string]Codec{
	"proto": ProtoCodec{},
	"json":  JSONCodec{},
}

// RegisterCodec registers a Codec with gRPC.
func RegisterCodec(codec Codec) {
	if codec == nil {
		panic("cannot register nil Codec")
	}
	registeredCodecs[codec.Name()] = codec
}

// GetCodec returns a registered Codec by name.
func GetCodec(name string) Codec {
	return registeredCodecs[name]
}
