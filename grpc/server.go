// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package grpc provides a compact, zero-allocation, high-performance gRPC server engine for Go.
//
// It is 100% wire-compatible with standard gRPC and protoc-gen-go-grpc generated services,
// supporting Unary, Server Streaming, Client Streaming, and Bidirectional Streaming RPCs.
package grpc

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"


	"github.com/lemon4ksan/sein/grpc/codes"
	"github.com/lemon4ksan/sein/grpc/metadata"
	"github.com/lemon4ksan/sein/grpc/status"
)

// ServerOption configures a gRPC server.
type ServerOption func(*Server)

// UnaryInterceptor returns a ServerOption that sets the UnaryServerInterceptor for the server.
func UnaryInterceptor(i UnaryServerInterceptor) ServerOption {
	return func(s *Server) {
		s.unaryInt = i
	}
}

// ChainUnaryInterceptor returns a ServerOption that specifies the chained interceptors for unary RPCs.
func ChainUnaryInterceptor(interceptors ...UnaryServerInterceptor) ServerOption {
	return func(s *Server) {
		s.unaryInt = chainUnaryInterceptors(interceptors)
	}
}

// StreamInterceptor returns a ServerOption that sets the StreamServerInterceptor for the server.
func StreamInterceptor(i StreamServerInterceptor) ServerOption {
	return func(s *Server) {
		s.streamInt = i
	}
}

// ChainStreamInterceptor returns a ServerOption that specifies the chained interceptors for streaming RPCs.
func ChainStreamInterceptor(interceptors ...StreamServerInterceptor) ServerOption {
	return func(s *Server) {
		s.streamInt = chainStreamInterceptors(interceptors)
	}
}

// MaxRecvMsgSize returns a ServerOption that sets the maximum message size the server can receive.
func MaxRecvMsgSize(m int) ServerOption {
	return func(s *Server) {
		s.maxRecvMsgSize = m
	}
}

// MaxSendMsgSize returns a ServerOption that sets the maximum message size the server can send.
func MaxSendMsgSize(m int) ServerOption {
	return func(s *Server) {
		s.maxSendMsgSize = m
	}
}

// CustomCodec returns a ServerOption that sets the default message codec.
func CustomCodec(c Codec) ServerOption {
	return func(s *Server) {
		s.codec = c
	}
}

type serviceInfo struct {
	impl    any
	methods map[string]*MethodDesc
	streams map[string]*StreamDesc
}

// Server is a compact, highload gRPC server.
type Server struct {
	mu             sync.RWMutex
	services       map[string]*serviceInfo
	unaryInt       UnaryServerInterceptor
	streamInt      StreamServerInterceptor
	codec          Codec
	maxRecvMsgSize int
	maxSendMsgSize int
	httpServer     *http.Server
	closed         bool
}

// NewServer creates a new gRPC server instance with the provided options.
func NewServer(opts ...ServerOption) *Server {
	s := &Server{
		services:       make(map[string]*serviceInfo),
		codec:          ProtoCodec{},
		maxRecvMsgSize: DefaultMaxReceiveMsgSize,
		maxSendMsgSize: DefaultMaxSendMsgSize,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// RegisterService registers a service and its implementation to the gRPC server.
func (s *Server) RegisterService(desc *ServiceDesc, impl any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if desc == nil || impl == nil {
		panic("grpc: desc and impl must not be nil")
	}

	sInfo := &serviceInfo{
		impl:    impl,
		methods: make(map[string]*MethodDesc, len(desc.Methods)),
		streams: make(map[string]*StreamDesc, len(desc.Streams)),
	}

	for i := range desc.Methods {
		d := &desc.Methods[i]
		sInfo.methods[d.MethodName] = d
	}

	for i := range desc.Streams {
		d := &desc.Streams[i]
		sInfo.streams[d.StreamName] = d
	}

	s.services[desc.ServiceName] = sInfo
}

// Serve accepts incoming connections on the listener ln, creating a new service goroutine for each.
func (s *Server) Serve(ln net.Listener) error {
	var p http.Protocols
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	p.SetHTTP2(true)

	s.mu.Lock()
	s.httpServer = &http.Server{
		Handler:           s,
		Protocols:         &p,
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.mu.Unlock()

	return s.httpServer.Serve(ln)
}

// ServeHTTP implements http.Handler, processing gRPC requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Validate gRPC Content-Type
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/grpc") {
		http.Error(w, "invalid content-type: must be application/grpc", http.StatusUnsupportedMediaType)
		return
	}

	// 2. Parse /Service/Method
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		writeStatus(w, status.New(codes.Unimplemented, "invalid gRPC method path"))
		return
	}

	serviceName, methodName := parts[0], parts[1]

	s.mu.RLock()
	sInfo, ok := s.services[serviceName]
	s.mu.RUnlock()

	if !ok {
		writeStatus(w, status.Newf(codes.Unimplemented, "unknown service %s", serviceName))
		return
	}

	// 3. Setup Incoming Metadata
	md := extractMetadata(r.Header)
	ctx := metadata.NewIncomingContext(r.Context(), md)
	ctx, sm := metadata.NewServerMetadataContext(ctx)

	// Check if unary method
	if mDesc, ok := sInfo.methods[methodName]; ok {
		s.handleUnary(w, r, ctx, sm, sInfo.impl, mDesc, path)
		return
	}

	// Check if streaming method
	if sDesc, ok := sInfo.streams[methodName]; ok {
		s.handleStream(w, r, ctx, sm, sInfo.impl, sDesc, path)
		return
	}

	writeStatus(w, status.Newf(codes.Unimplemented, "unknown method %s for service %s", methodName, serviceName))
}

func (s *Server) handleUnary(
	w http.ResponseWriter,
	r *http.Request,
	ctx context.Context,
	sm *metadata.ServerMetadataContext,
	impl any,
	desc *MethodDesc,
	fullMethod string,
) {
	dec := func(v any) error {
		data, _, err := ReadMsg(r.Body, s.maxRecvMsgSize)
		if err != nil {
			return err
		}

		if err := s.codec.Unmarshal(data, v); err != nil {
			return status.Errorf(codes.InvalidArgument, "failed to unmarshal request: %v", err)
		}

		return nil
	}

	var (
		resp any
		err  error
	)

	if s.unaryInt != nil {
		info := &UnaryServerInfo{
			Server:     impl,
			FullMethod: "/" + fullMethod,
		}
		handler := func(c context.Context, req any) (any, error) {
			return desc.Handler(impl, c, func(v any) error {
				// req was already decoded before calling interceptor chain if needed
				return nil
			}, nil)
		}
		resp, err = desc.Handler(impl, ctx, dec, s.unaryInt)
		_ = info
		_ = handler
	} else {
		resp, err = desc.Handler(impl, ctx, dec, nil)
	}

	if err != nil {
		writeStatusWithMetadata(w, status.Convert(err), sm)
		return
	}

	respBytes, err := s.codec.Marshal(resp)
	if err != nil {
		writeStatusWithMetadata(w, status.Newf(codes.Internal, "failed to marshal response: %v", err), sm)
		return
	}

	if s.maxSendMsgSize > 0 && len(respBytes) > s.maxSendMsgSize {
		writeStatusWithMetadata(w, status.Newf(codes.ResourceExhausted, "response message too large (%d > %d)", len(respBytes), s.maxSendMsgSize), sm)
		return
	}

	// 1. Set outgoing headers
	h := w.Header()
	h.Set("Content-Type", "application/grpc")
	sm.Header.CopyToHTTP(h)

	// 2. Set trailers before writing body (Go net/http trailer mechanism)
	h.Set("Trailer", "Grpc-Status, Grpc-Message")
	for k := range sm.Trailer {
		h.Add("Trailer", k)
	}

	w.WriteHeader(http.StatusOK)

	// 3. Write data frame
	if err := WriteMsg(w, respBytes, false); err != nil {
		return
	}

	// 4. Set final trailers (after WriteHeader)
	h.Set("Grpc-Status", strconv.Itoa(int(codes.OK)))
	h.Set("Grpc-Message", "")
	sm.Trailer.CopyToHTTP(h)
}

func (s *Server) handleStream(
	w http.ResponseWriter,
	r *http.Request,
	ctx context.Context,
	sm *metadata.ServerMetadataContext,
	impl any,
	desc *StreamDesc,
	fullMethod string,
) {
	ss := newServerStream(ctx, w, r.Body, s.codec, s.maxRecvMsgSize, s.maxSendMsgSize)

	var err error
	if s.streamInt != nil {
		info := &StreamServerInfo{
			FullMethod:     "/" + fullMethod,
			IsClientStream: desc.ClientStreams,
			IsServerStream: desc.ServerStreams,
		}
		handler := func(srv any, stream ServerStream) error {
			return desc.Handler(srv, stream)
		}
		err = s.streamInt(impl, ss, info, handler)
	} else {
		err = desc.Handler(impl, ss)
	}

	st := status.Convert(err)

	if !ss.headerSent {
		ss.writeHeaderLocked()
	}

	h := w.Header()
	h.Set("Grpc-Status", strconv.Itoa(int(st.Code())))
	if msg := st.Message(); msg != "" {
		h.Set("Grpc-Message", url.QueryEscape(msg))
	}
	ss.trailerMD.CopyToHTTP(h)
	sm.Trailer.CopyToHTTP(h)
}

// Stop stops the gRPC server. It immediately closes all open connections and listeners.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	if s.httpServer != nil {
		_ = s.httpServer.Close()
	}
}

// GracefulStop stops the gRPC server gracefully.
func (s *Server) GracefulStop(ctx ...context.Context) error {
	s.mu.Lock()
	s.closed = true
	srv := s.httpServer
	s.mu.Unlock()

	if srv != nil {
		c := context.Background()
		if len(ctx) > 0 && ctx[0] != nil {
			c = ctx[0]
		} else {
			var cancel context.CancelFunc
			c, cancel = context.WithTimeout(c, 10*time.Second)
			defer cancel()
		}

		return srv.Shutdown(c)
	}

	return nil
}

func extractMetadata(h http.Header) metadata.MD {
	md := make(metadata.MD, len(h))
	for k, vals := range h {
		k = strings.ToLower(k)
		if k == "content-type" || k == "user-agent" || k == "te" {
			continue
		}
		md[k] = append([]string(nil), vals...)
	}

	return md
}

func writeStatus(w http.ResponseWriter, st *status.Status) {
	writeStatusWithMetadata(w, st, nil)
}

func writeStatusWithMetadata(w http.ResponseWriter, st *status.Status, sm *metadata.ServerMetadataContext) {
	h := w.Header()
	h.Set("Content-Type", "application/grpc")
	h.Set("Grpc-Status", strconv.Itoa(int(st.Code())))
	if msg := st.Message(); msg != "" {
		h.Set("Grpc-Message", url.QueryEscape(msg))
	}

	if sm != nil {
		for k, vals := range sm.Header {
			for _, val := range vals {
				h.Add(k, val)
			}
		}
		for k, vals := range sm.Trailer {
			for _, val := range vals {
				h.Add(k, val)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func chainUnaryInterceptors(interceptors []UnaryServerInterceptor) UnaryServerInterceptor {
	n := len(interceptors)
	if n == 0 {
		return nil
	}
	if n == 1 {
		return interceptors[0]
	}

	return func(ctx context.Context, req any, info *UnaryServerInfo, handler UnaryHandler) (any, error) {
		currHandler := handler
		for i := n - 1; i >= 0; i-- {
			next := currHandler
			interceptor := interceptors[i]
			currHandler = func(c context.Context, r any) (any, error) {
				return interceptor(c, r, info, next)
			}
		}

		return currHandler(ctx, req)
	}
}

func chainStreamInterceptors(interceptors []StreamServerInterceptor) StreamServerInterceptor {
	n := len(interceptors)
	if n == 0 {
		return nil
	}
	if n == 1 {
		return interceptors[0]
	}

	return func(srv any, ss ServerStream, info *StreamServerInfo, handler StreamHandler) error {
		currHandler := handler
		for i := n - 1; i >= 0; i-- {
			next := currHandler
			interceptor := interceptors[i]
			currHandler = func(s any, stream ServerStream) error {
				return interceptor(s, stream, info, next)
			}
		}

		return currHandler(srv, ss)
	}
}
