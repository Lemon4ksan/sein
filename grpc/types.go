// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpc

import (
	"context"

	"github.com/lemon4ksan/sein/grpc/metadata"
)

// ServiceRegistrar wraps a single method that supports service registration.
// It allows custom gRPC server implementations to register generated proto services.
type ServiceRegistrar interface {
	RegisterService(desc *ServiceDesc, impl any)
}

// ServiceDesc represents an RPC service's specification.
type ServiceDesc struct {
	ServiceName string
	HandlerType any
	Methods     []MethodDesc
	Streams     []StreamDesc
	Metadata    any
}

// MethodDesc represents an RPC service's method specification.
type MethodDesc struct {
	MethodName string
	Handler    methodHandler
}

// StreamDesc represents an RPC service's stream specification.
type StreamDesc struct {
	StreamName    string
	Handler       streamHandler
	ServerStreams bool
	ClientStreams bool
}

type methodHandler func(srv any, ctx context.Context, dec func(any) error, interceptor UnaryServerInterceptor) (any, error)

type streamHandler func(srv any, stream ServerStream) error

// ServerStream defines the server-side behavior of a streaming RPC.
type ServerStream interface {
	// SetHeader sets the header metadata. It may be called multiple times.
	// When call multiple times, all the provided metadata will be merged.
	SetHeader(metadata.MD) error

	// SendHeader sends the header metadata.
	// The header metadata will be sent immediately.
	SendHeader(metadata.MD) error

	// SetTrailer sets the trailer metadata which will be sent with the RPC status.
	SetTrailer(metadata.MD)

	// Context returns the context for this stream.
	Context() context.Context

	// SendMsg sends a proto message.
	SendMsg(m any) error

	// RecvMsg blocks until it receives a proto message into m.
	RecvMsg(m any) error
}

// UnaryServerInfo provides information about a unary RPC.
type UnaryServerInfo struct {
	Server     any
	FullMethod string
}

// UnaryHandler defines the handler invoked by UnaryServerInterceptor to complete the normal
// execution of a unary RPC.
type UnaryHandler func(ctx context.Context, req any) (any, error)

// UnaryServerInterceptor provides a hook to intercept the execution of a unary RPC on the server.
type UnaryServerInterceptor func(ctx context.Context, req any, info *UnaryServerInfo, handler UnaryHandler) (resp any, err error)

// StreamServerInfo provides information about a streaming RPC.
type StreamServerInfo struct {
	FullMethod     string
	IsClientStream bool
	IsServerStream bool
}

// StreamHandler defines the handler invoked by StreamServerInterceptor to complete the normal
// execution of a streaming RPC.
type StreamHandler func(srv any, stream ServerStream) error

// StreamServerInterceptor provides a hook to intercept the execution of a streaming RPC on the server.
type StreamServerInterceptor func(srv any, ss ServerStream, info *StreamServerInfo, handler StreamHandler) error
