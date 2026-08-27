// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein/internal/fast/h1engine"
)

// StreamWriterResponse provides streaming chunked output to the client over HTTP/1.1.
type StreamWriterResponse struct {
	Status      int
	Headers     http.Header
	WriterFunc  func(w io.Writer) error
	ContentType string
}

// StreamWriter creates a streaming response executing fn.
func StreamWriter(fn func(w io.Writer) error) StreamWriterResponse {
	return StreamWriterResponse{
		Status:      http.StatusOK,
		WriterFunc:  fn,
		ContentType: header.MIMEApplicationOctetStream,
	}
}

// WithHeader attaches custom headers to the streaming response.
func (s StreamWriterResponse) WithHeader(key, val string) StreamWriterResponse {
	if s.Headers == nil {
		s.Headers = make(http.Header)
	}

	s.Headers.Set(key, val)

	return s
}

// WithContentType sets the Content-Type header on the stream.
func (s StreamWriterResponse) WithContentType(ct string) StreamWriterResponse {
	s.ContentType = ct
	return s
}

// WriteToH1 streams data directly into the connection socket buffer via chunked transfer encoding.
func (s StreamWriterResponse) WriteToH1(res *h1engine.Response) error {
	res.StatusCode = generic.Coalesce(s.Status, http.StatusOK)

	if s.ContentType != "" {
		res.Headers.Set(header.ContentType, s.ContentType)
	}

	res.Headers.AddFromHTTP(s.Headers)
	res.StreamWriter = s.WriterFunc

	return nil
}

// WriteResponse provides compatibility for net/http.
func (s StreamWriterResponse) WriteResponse(w http.ResponseWriter) error {
	if s.ContentType != "" {
		w.Header().Set(header.ContentType, s.ContentType)
	}

	copyHTTPHeaders(w.Header(), s.Headers)
	w.WriteHeader(generic.Coalesce(s.Status, http.StatusOK))

	flusher, _ := w.(http.Flusher)
	fw := &flushingWriter{w: w, flusher: flusher}

	if s.WriterFunc != nil {
		return s.WriterFunc(fw)
	}

	return nil
}

type flushingWriter struct {
	w       io.Writer
	flusher http.Flusher
}

func (fw *flushingWriter) Write(p []byte) (n int, err error) {
	n, err = fw.w.Write(p)
	if fw.flusher != nil {
		fw.flusher.Flush()
	}

	return n, err
}

// SSESender sends Server-Sent Events (SSE) according to the W3C EventSource standard.
//
// # RFC Compliance
//
// Conforms to the W3C Server-Sent Events specification (`text/event-stream`).
type SSESender struct {
	w io.Writer
}

// NewSSESender creates an [SSESender] wrapping the destination network writer.
func NewSSESender(w io.Writer) *SSESender {
	return &SSESender{w: w}
}

// Send emits a simple data-only SSE message.
//
// # Example
//
//	_ = sse.Send("hello client")
func (s *SSESender) Send(data string) error {
	var sb strings.Builder
	for line := range strings.SplitSeq(data, "\n") {
		sb.WriteString("data: ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	_, err := io.WriteString(s.w, sb.String())

	return err
}

// SendEvent emits a named event with string payload.
//
// # Example
//
//	_ = sse.SendEvent("price_update", `{"symbol":"BTC","price":98000}`)
func (s *SSESender) SendEvent(event, data string) error {
	var sb strings.Builder
	if event != "" {
		sb.WriteString("event: ")
		sb.WriteString(event)
		sb.WriteString("\n")
	}

	for line := range strings.SplitSeq(data, "\n") {
		sb.WriteString("data: ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	_, err := io.WriteString(s.w, sb.String())

	return err
}

// SendJSON emits a named event with an automatically serialized JSON payload.
//
// # Example
//
//	_ = sse.SendJSON("user_joined", User{ID: 42, Name: "Alice"})
func (s *SSESender) SendJSON(event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return s.SendEvent(event, string(data))
}

// SendComment emits a comment line (heartbeat/keepalive ping).
func (s *SSESender) SendComment(comment string) error {
	msg := fmt.Sprintf(": %s\n\n", comment)
	_, err := io.WriteString(s.w, msg)
	return err
}

// SendRetry advises the client reconnection backoff delay in milliseconds.
func (s *SSESender) SendRetry(ms int) error {
	msg := "retry: " + strconv.Itoa(ms) + "\n\n"
	_, err := io.WriteString(s.w, msg)
	return err
}

// SSEResponse encapsulates a Server-Sent Events real-time event stream.
type SSEResponse struct {
	StreamFunc func(sse *SSESender) error
	Headers    http.Header
}

// SSE creates an SSE streaming response handler.
//
// # Example
//
//	srv.Get("/events", func(ctx context.Context) (sein.SSEResponse, error) {
//	    return sein.SSE(func(sse *sein.SSESender) error {
//	        for i := 0; i < 5; i++ {
//	            _ = sse.SendJSON("tick", map[string]int{"count": i})
//	            time.Sleep(1 * time.Second)
//	        }
//	        return nil
//	    }), nil
//	})
func SSE(fn func(sse *SSESender) error) SSEResponse {
	return SSEResponse{
		StreamFunc: fn,
	}
}

// WithHeader attaches custom headers to the SSE response.
func (r SSEResponse) WithHeader(key, val string) SSEResponse {
	if r.Headers == nil {
		r.Headers = make(http.Header)
	}

	r.Headers.Set(key, val)

	return r
}

// WriteToH1 configures SSE headers and binds SSESender for direct H1 delivery.
func (r SSEResponse) WriteToH1(res *h1engine.Response) error {
	res.StatusCode = http.StatusOK
	res.Headers.Set(header.ContentType, "text/event-stream")
	res.Headers.Set(header.CacheControl, "no-cache")
	res.Headers.Set(header.Connection, "keep-alive")

	res.Headers.AddFromHTTP(r.Headers)

	res.StreamWriter = func(w io.Writer) error {
		if r.StreamFunc != nil {
			return r.StreamFunc(NewSSESender(w))
		}

		return nil
	}

	return nil
}

// WriteResponse satisfies net/http Responder.
func (r SSEResponse) WriteResponse(w http.ResponseWriter) error {
	w.Header().Set(header.ContentType, "text/event-stream")
	w.Header().Set(header.CacheControl, "no-cache")
	w.Header().Set(header.Connection, "keep-alive")

	copyHTTPHeaders(w.Header(), r.Headers)
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	fw := &flushingWriter{w: w, flusher: flusher}
	sse := NewSSESender(fw)

	if r.StreamFunc != nil {
		return r.StreamFunc(sse)
	}

	return nil
}
