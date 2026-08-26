// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

var (
	readerPool = sync.Pool{
		New: func() any {
			return bufio.NewReaderSize(nil, 4096)
		},
	}
	writerPool = sync.Pool{
		New: func() any {
			return bufio.NewWriterSize(nil, 4096)
		},
	}
	reqPool = sync.Pool{
		New: func() any {
			return &Request{
				Body: make([]byte, 0, 1024),
			}
		},
	}
	resPool = sync.Pool{
		New: func() any {
			return &Response{
				Body: make([]byte, 0, 1024),
			}
		},
	}
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
	var isHijacked bool
	defer func() {
		if !isHijacked {
			_ = conn.Close()
		}
	}()

	br := readerPool.Get().(*bufio.Reader)
	br.Reset(conn)
	defer readerPool.Put(br)

	bw := writerPool.Get().(*bufio.Writer)
	bw.Reset(conn)
	defer writerPool.Put(bw)

	req := reqPool.Get().(*Request)
	defer reqPool.Put(req)

	res := resPool.Get().(*Response)
	defer resPool.Put(res)

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

		if err := res.WriteTo(bw, keepAlive); err != nil {
			return err
		}

		if !keepAlive {
			return nil
		}
	}
}
