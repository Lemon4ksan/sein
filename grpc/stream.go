// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package grpc

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/lemon4ksan/sein/grpc/codes"
	"github.com/lemon4ksan/sein/grpc/metadata"
	"github.com/lemon4ksan/sein/grpc/status"
)

type serverStream struct {
	ctx               context.Context
	w                 http.ResponseWriter
	r                 io.Reader
	flusher           http.Flusher
	codec             Codec
	maxReceiveMsgSize int
	maxSendMsgSize    int

	mu         sync.Mutex
	headerSent bool
	headerMD   metadata.MD
	trailerMD  metadata.MD
}

func newServerStream(
	ctx context.Context,
	w http.ResponseWriter,
	r io.Reader,
	codec Codec,
	maxRecvSize, maxSendSize int,
) *serverStream {
	var flusher http.Flusher
	if f, ok := w.(http.Flusher); ok {
		flusher = f
	}

	return &serverStream{
		ctx:               ctx,
		w:                 w,
		r:                 r,
		flusher:           flusher,
		codec:             codec,
		maxReceiveMsgSize: maxRecvSize,
		maxSendMsgSize:    maxSendSize,
		headerMD:          make(metadata.MD),
		trailerMD:         make(metadata.MD),
	}
}

func (s *serverStream) Context() context.Context {
	return s.ctx
}

func (s *serverStream) SetHeader(md metadata.MD) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.headerSent {
		return errors.New("grpc: headers already sent")
	}

	for k, v := range md {
		s.headerMD.Append(k, v...)
	}

	return nil
}

func (s *serverStream) SendHeader(md metadata.MD) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.headerSent {
		return errors.New("grpc: headers already sent")
	}

	for k, v := range md {
		s.headerMD.Append(k, v...)
	}

	s.writeHeaderLocked()

	return nil
}

func (s *serverStream) writeHeaderLocked() {
	if s.headerSent {
		return
	}
	s.headerSent = true

	h := s.w.Header()
	h.Set("Content-Type", "application/grpc")
	h.Set("Trailer", "Grpc-Status, Grpc-Message")
	s.headerMD.CopyToHTTP(h)

	s.w.WriteHeader(http.StatusOK)
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func (s *serverStream) SetTrailer(md metadata.MD) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for k, v := range md {
		s.trailerMD.Append(k, v...)
	}
}

func (s *serverStream) SendMsg(m any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.headerSent {
		s.writeHeaderLocked()
	}

	data, err := s.codec.Marshal(m)
	if err != nil {
		return status.Errorf(codes.Internal, "grpc: failed to marshal response: %v", err)
	}

	if s.maxSendMsgSize > 0 && len(data) > s.maxSendMsgSize {
		return status.Errorf(codes.ResourceExhausted, "grpc: response message too large (%d > %d)", len(data), s.maxSendMsgSize)
	}

	if err := WriteMsg(s.w, data, false); err != nil {
		return err
	}

	if s.flusher != nil {
		s.flusher.Flush()
	}

	return nil
}

func (s *serverStream) RecvMsg(m any) error {
	data, _, err := ReadMsg(s.r, s.maxReceiveMsgSize)
	if err != nil {
		return err
	}

	if err := s.codec.Unmarshal(data, m); err != nil {
		return status.Errorf(codes.InvalidArgument, "grpc: failed to unmarshal request: %v", err)
	}

	return nil
}
