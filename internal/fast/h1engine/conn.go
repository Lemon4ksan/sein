// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1engine

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"time"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/silicon/pool"
)

var (
	readerStorage = pool.NewPerPStorage(func() *bufio.Reader {
		return bufio.NewReaderSize(nil, 4096)
	})
	writerStorage = pool.NewPerPStorage(func() *bufio.Writer {
		return bufio.NewWriterSize(nil, 4096)
	})
	reqStorage = pool.NewPerPStorage(func() *Request {
		return &Request{
			Body: make([]byte, 0, 1024),
			Headers: Headers{
				entries: make([]HeaderEntry, 0, 16),
			},
		}
	})
	resStorage = pool.NewPerPStorage(func() *Response {
		return &Response{
			Body: make([]byte, 0, 1024),
		}
	})
)

// HandlerFunc is the core callback for dispatching an incoming H1 request to the server router.
type HandlerFunc func(req *Request, res *Response) error

// ConnHandler manages the lifecycle of a single incoming TCP or TLS connection.
type ConnHandler struct {
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	MaxBodySize  int64
	Handler      HandlerFunc
}

// ServeConn processes HTTP/1.1 requests sequentially on conn until closed or error occurs.
func (ch *ConnHandler) ServeConn(conn net.Conn) error {
	br := readerStorage.Get()
	br.Reset(conn)

	bw := writerStorage.Get()
	bw.Reset(conn)

	var isHijacked bool
	defer func() {
		if !isHijacked {
			_ = bw.Flush()
			_ = conn.Close()
			readerStorage.Put(br)
			writerStorage.Put(bw)
		}
	}()

	req := reqStorage.Get()
	defer reqStorage.Put(req)

	res := resStorage.Get()
	defer resStorage.Put(res)

	remoteAddr := conn.RemoteAddr().String()

	var tlsState *tls.ConnectionState
	if tlsConn, ok := conn.(*tls.Conn); ok {
		state := tlsConn.ConnectionState()
		tlsState = &state
	}

	maxBody := ch.MaxBodySize
	if maxBody <= 0 {
		maxBody = 32 << 20 // 32MB default
	}

	hijackFn := func() (net.Conn, *bufio.ReadWriter, error) {
		if isHijacked {
			return nil, nil, errors.New("h1: connection already hijacked")
		}

		isHijacked = true
		rw := bufio.NewReadWriter(br, bw)

		return conn, rw, nil
	}

	for {
		req.Reset()
		res.Reset()

		req.RemoteAddr = remoteAddr
		req.TLS = tlsState
		req.HijackFn = hijackFn

		// Set read timeout
		if ch.ReadTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(ch.ReadTimeout))
		} else if ch.IdleTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(ch.IdleTimeout))
		}

		err := req.ReadRequest(br, bw, maxBody)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}

			return err
		}

		keepAlive := req.Headers.IsKeepAlive(req.Proto)
		// RFC 9112 §6.3 Item 3 & §11.2: To mitigate Request Smuggling when both Transfer-Encoding
		// and Content-Length were received, the server MUST close the connection after responding.
		if req.Headers.Has(header.TransferEncoding) && req.Headers.Has(header.ContentLength) {
			keepAlive = false
		}

		// Set write timeout
		if ch.WriteTimeout > 0 {
			_ = conn.SetWriteDeadline(time.Now().Add(ch.WriteTimeout))
		}

		if ch.Handler != nil {
			if err := ch.Handler(req, res); err != nil {
				res.StatusCode = 500
				res.Body = []byte(`{"error":"INTERNAL_ERROR","message":"Internal Server Error"}`)
			}
		}

		if isHijacked {
			// Handler took over the raw connection
			return nil
		}

		// High-Throughput HTTP Pipelining Coalescing:
		// If more unread request bytes already reside in br and connection is keep-alive,
		// defer bw.Flush() to combine multiple pipelined responses into a single TCP write.
		hasMorePipelined := keepAlive && br.Buffered() > 0

		if err := res.WriteTo(bw, keepAlive, !hasMorePipelined); err != nil {
			return err
		}

		if !keepAlive {
			return nil
		}
	}
}
