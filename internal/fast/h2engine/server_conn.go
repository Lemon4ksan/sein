// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2engine

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/silicon/pool"
)

// ServerHandlerFunc is the callback signature for dispatching an incoming H2 stream request.
type ServerHandlerFunc func(req *ServerRequest, res *ServerResponse) error

// ServerRequest represents a parsed incoming HTTP/2 stream request.
type ServerRequest struct {
	StreamID   uint32
	Method     string
	Path       string
	Scheme     string
	Authority  string
	Protocol   string
	Headers    http.Header
	Body       []byte
	RemoteAddr string
	Ctx        context.Context
}

// ServerResponse represents an outgoing HTTP/2 stream response.
type ServerResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

type serverStream struct {
	id          uint32
	method      string
	path        string
	scheme      string
	authority   string
	protocol    string
	headers     http.Header
	headerBlock bytes.Buffer
	body        bytes.Buffer
	endHeaders  bool
	endStream   bool
}

// ServerConn manages a single server-side HTTP/2 connection.
type ServerConn struct {
	conn       net.Conn
	br         *bufio.Reader
	bw         *bufio.Writer
	handler    ServerHandlerFunc
	hpackDec   *HPACK
	hpackEnc   *HPACK
	encMu      sync.Mutex
	writeMu    sync.Mutex
	streamsMu  sync.RWMutex
	streams    map[uint32]*serverStream
	isClosed   atomic.Bool
	closeErr   error

	peerMaxFrameSize uint32
	peerInitialWin   int32
}

var serverConnStorage = pool.NewPerPStorage(func() *ServerConn {
	return &ServerConn{
		hpackDec:         AcquireHPACK(),
		hpackEnc:         AcquireHPACK(),
		streams:          make(map[uint32]*serverStream, 64),
		peerMaxFrameSize: defaultMaxLen,
		peerInitialWin:   65535,
	}
})

// NewServerConn creates a new HTTP/2 server connection handler wrapping netConn.
func NewServerConn(netConn net.Conn, handler ServerHandlerFunc) *ServerConn {
	sc := serverConnStorage.Get()
	sc.conn = netConn
	sc.br = bufio.NewReaderSize(netConn, 4096)
	sc.bw = bufio.NewWriterSize(netConn, 4096)
	sc.handler = handler
	sc.isClosed.Store(false)
	sc.closeErr = nil
	sc.peerMaxFrameSize = defaultMaxLen
	sc.peerInitialWin = 65535
	sc.hpackDec.Reset()
	sc.hpackEnc.Reset()
	sc.hpackEnc.DisableDynamicTable = true
	clear(sc.streams)
	return sc
}

// Release returns the ServerConn to the core pool.
func (sc *ServerConn) Release() {
	sc.isClosed.Store(true)
	clear(sc.streams)
	serverConnStorage.Put(sc)
}

// Serve runs the main HTTP/2 server connection loop.
func (sc *ServerConn) Serve() error {
	defer sc.conn.Close()

	// 1. Read and verify 24-byte client connection preface (RFC 9113 §3.4)
	if !ReadPreface(sc.br) {
		return errors.New("h2: invalid connection preface")
	}

	// 2. Send initial server SETTINGS frame
	st := &Settings{}
	st.SetMaxConcurrentStreams(1000)
	st.SetMaxFrameSize(defaultMaxLen)
	st.SetMaxWindowSize(65535)

	if err := sc.sendSettings(st, false); err != nil {
		return err
	}

	// 3. Main frame reading loop
	for {
		if sc.isClosed.Load() {
			return sc.closeErr
		}

		fr, err := ReadFrameFrom(sc.br)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}

		switch fr.Type() {
		case FrameSettings:
			if err := sc.handleSettings(fr); err != nil {
				ReleaseFrameHeader(fr)
				return err
			}

		case FramePing:
			if err := sc.handlePing(fr); err != nil {
				ReleaseFrameHeader(fr)
				return err
			}

		case FrameHeaders:
			if err := sc.handleHeaders(fr); err != nil {
				ReleaseFrameHeader(fr)
				return err
			}

		case FrameContinuation:
			if err := sc.handleContinuation(fr); err != nil {
				ReleaseFrameHeader(fr)
				return err
			}

		case FrameData:
			if err := sc.handleData(fr); err != nil {
				ReleaseFrameHeader(fr)
				return err
			}

		case FrameWindowUpdate:
			// Flow control window updates
			ReleaseFrameHeader(fr)

		case FrameResetStream:
			sc.streamsMu.Lock()
			delete(sc.streams, fr.Stream())
			sc.streamsMu.Unlock()
			ReleaseFrameHeader(fr)

		case FrameGoAway:
			ReleaseFrameHeader(fr)
			return nil

		default:
			ReleaseFrameHeader(fr)
		}
	}
}

func (sc *ServerConn) sendSettings(st *Settings, ack bool) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	fr := AcquireFrameHeader()
	defer ReleaseFrameHeader(fr)

	stFrame := AcquireFrame(FrameSettings).(*Settings)
	if ack {
		stFrame.SetAck(true)
		fr.SetFlags(FlagAck)
	} else {
		st.CopyTo(stFrame)
	}
	fr.SetBody(stFrame)

	if _, err := fr.WriteTo(sc.bw); err != nil {
		return err
	}
	return sc.bw.Flush()
}

func (sc *ServerConn) handleSettings(fr *FrameHeader) error {
	defer ReleaseFrameHeader(fr)

	if fr.Flags().Has(FlagAck) {
		// Client ACKed our settings
		return nil
	}

	// Apply peer settings
	if body := fr.Body(); body != nil {
		if st, ok := body.(*Settings); ok {
			if mfs := st.MaxFrameSize(); mfs >= 16384 && mfs <= 16777215 {
				sc.peerMaxFrameSize = mfs
			}
			if iws := st.MaxWindowSize(); iws > 0 {
				sc.peerInitialWin = int32(iws)
			}
		}
	}

	// Send Settings ACK
	return sc.sendSettings(nil, true)
}

func (sc *ServerConn) handlePing(fr *FrameHeader) error {
	defer ReleaseFrameHeader(fr)

	if fr.Flags().Has(FlagAck) {
		return nil
	}

	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	ackFr := AcquireFrameHeader()
	defer ReleaseFrameHeader(ackFr)

	ping := fr.Body().(*Ping)
	ackPing := AcquireFrame(FramePing).(*Ping)
	ackPing.SetData(ping.Data())

	ackFr.SetFlags(FlagAck)
	ackFr.SetBody(ackPing)

	if _, err := ackFr.WriteTo(sc.bw); err != nil {
		return err
	}
	return sc.bw.Flush()
}

func (sc *ServerConn) handleHeaders(fr *FrameHeader) error {
	defer ReleaseFrameHeader(fr)

	streamID := fr.Stream()
	// RFC 9113 §5.1.1: Client-initiated streams MUST use non-zero, odd-numbered stream identifiers
	if streamID == 0 || (streamID%2) == 0 {
		return ProtocolError
	}

	hFrame := fr.Body().(*Headers)
	endHeaders := fr.Flags().Has(FlagEndHeaders)
	endStream := fr.Flags().Has(FlagEndStream)

	st := &serverStream{
		id:         streamID,
		headers:    make(http.Header),
		endHeaders: endHeaders,
		endStream:  endStream,
	}

	// Write raw header fragment
	st.headerBlock.Write(hFrame.Headers())

	sc.streamsMu.Lock()
	sc.streams[streamID] = st
	sc.streamsMu.Unlock()

	if endHeaders {
		return sc.finishHeaderBlock(st)
	}

	return nil
}

func (sc *ServerConn) handleContinuation(fr *FrameHeader) error {
	defer ReleaseFrameHeader(fr)

	streamID := fr.Stream()
	sc.streamsMu.RLock()
	st, ok := sc.streams[streamID]
	sc.streamsMu.RUnlock()

	if !ok {
		return errors.New("h2: CONTINUATION on unknown stream (RFC 9113 §6.10)")
	}

	cFrame := fr.Body().(*Continuation)
	st.headerBlock.Write(cFrame.Headers())

	if fr.Flags().Has(FlagEndHeaders) {
		st.endHeaders = true
		return sc.finishHeaderBlock(st)
	}

	return nil
}

func (sc *ServerConn) finishHeaderBlock(st *serverStream) error {
	rawBlock := st.headerBlock.Bytes()
	hf := AcquireHeaderField()
	defer ReleaseHeaderField(hf)

	var hasSeenRegularHeader bool
	for len(rawBlock) > 0 {
		hf.Reset()
		var err error
		rawBlock, err = sc.hpackDec.Next(hf, rawBlock)
		if err != nil {
			// RFC 7541 & RFC 9113 §4.3: HPACK decoding errors MUST be treated as COMPRESSION_ERROR
			return CompressionError
		}
		if hf.Empty() {
			continue
		}

		k := string(hf.KeyBytes())
		v := string(hf.ValueBytes())

		// RFC 9113 §8.2: All field names MUST be lowercase ASCII
		for i := 0; i < len(k); i++ {
			if k[i] >= 'A' && k[i] <= 'Z' {
				return ProtocolError
			}
		}

		if hf.IsPseudo() {
			// RFC 9113 §8.3: Pseudo-headers MUST appear before regular headers
			if hasSeenRegularHeader {
				return ProtocolError
			}
			switch k {
			case ":method":
				if st.method != "" {
					return ProtocolError
				}
				st.method = v
			case ":path":
				if st.path != "" {
					return ProtocolError
				}
				st.path = v
			case ":scheme":
				if st.scheme != "" {
					return ProtocolError
				}
				st.scheme = v
			case ":authority":
				if st.authority != "" {
					return ProtocolError
				}
				st.authority = v
			case ":protocol":
				// RFC 8441 §4: Extended CONNECT pseudo-header
				if st.protocol != "" {
					return ProtocolError
				}
				st.protocol = v
			default:
				// RFC 9113 §8.3: Unknown or invalid pseudo-header
				return ProtocolError
			}
		} else {
			hasSeenRegularHeader = true

			// RFC 9113 §8.2.2: Connection-specific headers are prohibited in HTTP/2
			switch k {
			case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade":
				return ProtocolError
			case "te":
				if v != "trailers" {
					return ProtocolError
				}
			}

			st.headers.Add(k, v)
		}
	}

	// RFC 9113 §8.3.1 & RFC 8441 §4: Mandatory request pseudo-headers
	if st.method == "" {
		return ProtocolError
	}
	if st.protocol != "" {
		// RFC 8441 §4: :protocol pseudo-header is only valid on CONNECT requests with :scheme and :path
		if st.method != "CONNECT" || st.scheme == "" || st.path == "" {
			return ProtocolError
		}
	} else if st.method != "CONNECT" && (st.scheme == "" || st.path == "") {
		return ProtocolError
	}

	if st.endStream {
		go sc.dispatchStream(st)
	}

	return nil
}

func (sc *ServerConn) handleData(fr *FrameHeader) error {
	defer ReleaseFrameHeader(fr)

	streamID := fr.Stream()
	sc.streamsMu.RLock()
	st, ok := sc.streams[streamID]
	sc.streamsMu.RUnlock()

	if !ok {
		return nil
	}

	dFrame := fr.Body().(*Data)
	st.body.Write(dFrame.Data())

	if fr.Flags().Has(FlagEndStream) {
		st.endStream = true
		go sc.dispatchStream(st)
	}

	return nil
}

func (sc *ServerConn) dispatchStream(st *serverStream) {
	req := &ServerRequest{
		StreamID:   st.id,
		Method:     st.method,
		Path:       st.path,
		Scheme:     st.scheme,
		Authority:  st.authority,
		Protocol:   st.protocol,
		Headers:    st.headers,
		Body:       st.body.Bytes(),
		RemoteAddr: sc.conn.RemoteAddr().String(),
		Ctx:        context.Background(),
	}

	res := &ServerResponse{
		StatusCode: http.StatusOK,
		Headers:    make(http.Header),
	}

	if sc.handler != nil {
		_ = sc.handler(req, res)
	}

	_ = sc.writeResponse(st.id, res)

	sc.streamsMu.Lock()
	delete(sc.streams, st.id)
	sc.streamsMu.Unlock()
}

func (sc *ServerConn) writeResponse(streamID uint32, res *ServerResponse) error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()

	hdrFr := AcquireFrameHeader()
	defer ReleaseFrameHeader(hdrFr)

	sc.encMu.Lock()
	hFrame := AcquireFrame(FrameHeaders).(*Headers)
	SerializeResponseHeaders(hFrame, sc.hpackEnc, res.StatusCode, res.Headers, len(res.Body))
	sc.encMu.Unlock()

	hdrFr.SetStream(streamID)
	hdrFr.SetFlags(FlagEndHeaders)
	if len(res.Body) == 0 {
		hdrFr.SetFlags(FlagEndHeaders | FlagEndStream)
	}
	hdrFr.SetBody(hFrame)

	if _, err := hdrFr.WriteTo(sc.bw); err != nil {
		return err
	}

	// 2. Serialize DATA Frames
	body := res.Body
	maxChunk := int(sc.peerMaxFrameSize)
	if maxChunk <= 0 {
		maxChunk = defaultMaxLen
	}

	for len(body) > 0 {
		chunkSize := min(len(body), maxChunk)
		chunk := body[:chunkSize]
		body = body[chunkSize:]

		dataFr := AcquireFrameHeader()
		dFrame := AcquireFrame(FrameData).(*Data)
		dFrame.SetData(chunk)

		dataFr.SetStream(streamID)
		if len(body) == 0 {
			dataFr.SetFlags(FlagEndStream)
		}
		dataFr.SetBody(dFrame)

		_, err := dataFr.WriteTo(sc.bw)
		ReleaseFrameHeader(dataFr)

		if err != nil {
			return err
		}
	}

	return sc.bw.Flush()
}
