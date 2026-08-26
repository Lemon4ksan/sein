// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/lemon4ksan/sein/internal/quic"
	"github.com/lemon4ksan/sein/internal/quic/quicvarint"
)

// ServerHandlerFunc is the callback signature for dispatching an incoming H3 stream request.
type ServerHandlerFunc func(req *ServerRequest, res *ServerResponse) error

// ServerRequest represents a parsed incoming HTTP/3 request.
type ServerRequest struct {
	StreamID   uint64
	Method     string
	Path       string
	Scheme     string
	Authority  string
	Headers    http.Header
	Body       []byte
	RemoteAddr string
	Ctx        context.Context
}

// ServerResponse represents an outgoing HTTP/3 response.
type ServerResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// ServerConn manages an active HTTP/3 server connection over an underlying QUIC connection (RFC 9114).
type ServerConn struct {
	quicConn   *quic.Conn
	handler    ServerHandlerFunc
	qpack      *QPACKCodec
	isClosed   atomic.Bool
	closeErr   error
	controlOut *quic.SendStream
	writeMu    sync.Mutex
}

// NewServerConn creates a new HTTP/3 server connection wrapping a QUIC connection.
func NewServerConn(quicConn *quic.Conn, handler ServerHandlerFunc) *ServerConn {
	return &ServerConn{
		quicConn: quicConn,
		handler:  handler,
		qpack:    NewQPACKCodec(),
	}
}

// Serve initializes control streams and handles incoming bidirectional request streams.
func (sc *ServerConn) Serve() error {
	ctx := sc.quicConn.Context()

	// 1. Open and initialize server unidirectional control stream (RFC 9114 §6.2.1)
	ctrlStream, err := sc.quicConn.OpenUniStreamSync(ctx)
	if err != nil {
		return err
	}
	sc.controlOut = ctrlStream

	// Write Control Stream Type (0x00)
	var typeBuf [8]byte
	n := quicvarint.Append(typeBuf[:0], StreamTypeControl)
	if _, err := ctrlStream.Write(n); err != nil {
		return err
	}

	// Write initial server SETTINGS frame (RFC 9114 §7.2.4)
	st := &Settings{
		MaxFieldSectionSize: 64 * 1024,
	}
	if _, err := ctrlStream.Write(st.Encode()); err != nil {
		return err
	}

	// 2. Accept client-initiated unidirectional streams (Client control & QPACK streams) in background
	go sc.acceptUniStreams()

	// 3. Main loop: accept client-initiated bidirectional request streams (RFC 9114 §6.1)
	for {
		if sc.isClosed.Load() {
			return sc.closeErr
		}

		stream, err := sc.quicConn.AcceptStream(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		go sc.handleRequestStream(stream)
	}
}

func (sc *ServerConn) acceptUniStreams() {
	ctx := sc.quicConn.Context()
	for {
		stream, err := sc.quicConn.AcceptUniStream(ctx)
		if err != nil {
			return
		}
		go sc.handleUniStream(stream)
	}
}

func (sc *ServerConn) handleUniStream(stream *quic.ReceiveStream) {
	defer stream.CancelRead(0)

	// Read stream type varint
	qr := quicvarint.NewReader(stream)
	streamType, err := quicvarint.Read(qr)
	if err != nil {
		return
	}

	switch streamType {
	case StreamTypeControl:
		// Read peer settings frame
		frameType, err := quicvarint.Read(qr)
		if err != nil || frameType != FrameTypeSettings {
			return
		}
		frameLen, err := quicvarint.Read(qr)
		if err != nil {
			return
		}
		settingsPayload := make([]byte, frameLen)
		_, _ = io.ReadFull(stream, settingsPayload)

	case StreamTypeQPACKEncoder, StreamTypeQPACKDecoder:
		// Drain stream
		_, _ = io.Copy(io.Discard, stream)
	}
}

func (sc *ServerConn) handleRequestStream(stream *quic.Stream) {
	defer stream.Close()

	qr := quicvarint.NewReader(stream)
	var (
		headerBlock []byte
		bodyBuf     bytes.Buffer
	)

	// Read frames on request stream (RFC 9114 §7.1)
	for {
		frameType, err := quicvarint.Read(qr)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return
		}

		frameLen, err := quicvarint.Read(qr)
		if err != nil {
			return
		}

		switch frameType {
		case FrameTypeHeaders:
			headerBlock = make([]byte, frameLen)
			if _, err := io.ReadFull(stream, headerBlock); err != nil {
				return
			}

		case FrameTypeData:
			if frameLen > 0 {
				lr := io.LimitReader(stream, int64(frameLen))
				if _, err := io.Copy(&bodyBuf, lr); err != nil {
					return
				}
			}

		default:
			// Skip unknown frame
			if frameLen > 0 {
				lr := io.LimitReader(stream, int64(frameLen))
				_, _ = io.Copy(io.Discard, lr)
			}
		}
	}

	if len(headerBlock) == 0 {
		return
	}

	// Decode QPACK headers
	method, path, scheme, authority, headers, err := sc.qpack.DecodeRequestHeaders(headerBlock)
	if err != nil {
		return
	}

	req := &ServerRequest{
		StreamID:   uint64(stream.StreamID()),
		Method:     method,
		Path:       path,
		Scheme:     scheme,
		Authority:  authority,
		Headers:    headers,
		Body:       bodyBuf.Bytes(),
		RemoteAddr: sc.quicConn.RemoteAddr().String(),
		Ctx:        context.Background(),
	}

	res := &ServerResponse{
		StatusCode: http.StatusOK,
		Headers:    make(http.Header),
	}

	if sc.handler != nil {
		_ = sc.handler(req, res)
	}

	// Write response HEADERS frame
	respBlock := sc.qpack.EncodeResponseHeaders(res.StatusCode, res.Headers, len(res.Body))

	var frameHdr [16]byte
	hdrBytes := quicvarint.Append(frameHdr[:0], FrameTypeHeaders)
	hdrBytes = quicvarint.Append(hdrBytes, uint64(len(respBlock)))

	if _, err := stream.Write(hdrBytes); err != nil {
		return
	}
	if _, err := stream.Write(respBlock); err != nil {
		return
	}

	// Write response DATA frame
	if len(res.Body) > 0 {
		dataHdrBytes := quicvarint.Append(frameHdr[:0], FrameTypeData)
		dataHdrBytes = quicvarint.Append(dataHdrBytes, uint64(len(res.Body)))

		if _, err := stream.Write(dataHdrBytes); err != nil {
			return
		}
		if _, err := stream.Write(res.Body); err != nil {
			return
		}
	}
}

// Close gracefully closes the HTTP/3 connection.
func (sc *ServerConn) Close() error {
	sc.isClosed.Store(true)
	return sc.quicConn.CloseWithError(0x0100, "h3 normal closure")
}
