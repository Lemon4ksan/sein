// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sse

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein/internal/fast/h1engine"
)

// Event represents an individual W3C Server-Sent Event message.
type Event struct {
	// ID is the event identifier (sets the Last-Event-ID for reconnection).
	ID string

	// Event is the event type name (e.g. "message", "chunk", "error").
	Event string

	// Data is the message payload (string, []byte, or JSON object).
	Data any

	// Comment is a comment line (": ping") used for keep-alive heartbeats.
	Comment string

	// Retry is the reconnection time duration advertised to the client.
	Retry time.Duration
}

// Writer provides high-throughput streaming methods for sending Server-Sent Events to a client.
type Writer struct {
	w       io.Writer
	flusher http.Flusher
}

// NewWriter creates an SSE Writer wrapping w.
func NewWriter(w io.Writer) *Writer {
	flusher, _ := w.(http.Flusher)

	return &Writer{
		w:       w,
		flusher: flusher,
	}
}

// Send sends a default "data: ..." event message.
func (w *Writer) Send(data any) error {
	return w.SendEvent("", data)
}

// SendEvent sends a typed event with an event name and data payload.
func (w *Writer) SendEvent(event string, data any) error {
	var buf []byte

	if event != "" {
		buf = append(buf, "event: "...)
		buf = append(buf, event...)
		buf = append(buf, '\n')
	}

	switch v := data.(type) {
	case nil:
		// empty data
	case string:
		lines := strings.Split(v, "\n")
		for _, line := range lines {
			buf = append(buf, "data: "...)
			buf = append(buf, line...)
			buf = append(buf, '\n')
		}

	case []byte:
		lines := strings.Split(string(v), "\n")
		for _, line := range lines {
			buf = append(buf, "data: "...)
			buf = append(buf, line...)
			buf = append(buf, '\n')
		}

	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return err
		}

		buf = append(buf, "data: "...)
		buf = append(buf, encoded...)
		buf = append(buf, '\n')
	}

	buf = append(buf, '\n')

	_, err := w.w.Write(buf)
	if err != nil {
		return err
	}

	return w.Flush()
}

// SendComment sends a comment line (e.g. ": heartbeat\n\n") to keep the connection alive.
func (w *Writer) SendComment(comment string) error {
	msg := fmt.Sprintf(": %s\n\n", comment)

	_, err := w.w.Write([]byte(msg))
	if err != nil {
		return err
	}

	return w.Flush()
}

// SendID sends an event with an ID and data payload.
func (w *Writer) SendID(id string, data any) error {
	var buf []byte
	if id != "" {
		buf = append(buf, "id: "...)
		buf = append(buf, id...)
		buf = append(buf, '\n')
	}

	if data != nil {
		buf = append(buf, "data: "...)
		switch v := data.(type) {
		case string:
			buf = append(buf, v...)
		case []byte:
			buf = append(buf, v...)
		default:
			enc, _ := json.Marshal(v)
			buf = append(buf, enc...)
		}

		buf = append(buf, '\n')
	}

	buf = append(buf, '\n')

	_, err := w.w.Write(buf)
	if err != nil {
		return err
	}

	return w.Flush()
}

// SendRetry sends a client reconnection retry interval in milliseconds.
func (w *Writer) SendRetry(duration time.Duration) error {
	millis := strconv.FormatInt(duration.Milliseconds(), 10)
	msg := fmt.Sprintf("retry: %s\n\n", millis)

	_, err := w.w.Write([]byte(msg))
	if err != nil {
		return err
	}

	return w.Flush()
}

// Flush flushes buffered data downstream to the client.
func (w *Writer) Flush() error {
	if w.flusher != nil {
		w.flusher.Flush()
	}

	return nil
}

// StreamHandler is a streaming callback that receives an SSE Writer to stream events.
type StreamHandler func(w *Writer) error

// SSEStreamResponse implements sein.Responder and sein.DirectH1Responder for streaming SSE.
type SSEStreamResponse struct {
	handler StreamHandler
}

// WriteResponse streams SSE events via stdlib http.ResponseWriter.
func (s SSEStreamResponse) WriteResponse(w http.ResponseWriter) error {
	w.Header().Set(header.ContentType, "text/event-stream")
	w.Header().Set(header.CacheControl, "no-cache")
	w.Header().Set(header.Connection, "keep-alive")
	w.Header().Set(header.XAccelBuffering, "no")
	w.WriteHeader(http.StatusOK)

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	writer := NewWriter(w)

	return s.handler(writer)
}

// WriteToH1 streams SSE events via native H1 response.
func (s SSEStreamResponse) WriteToH1(res *h1engine.Response) error {
	res.Headers.Set(header.ContentType, "text/event-stream")
	res.Headers.Set(header.CacheControl, "no-cache")
	res.Headers.Set(header.Connection, "keep-alive")
	res.Headers.Set(header.XAccelBuffering, "no")
	res.StatusCode = http.StatusOK

	res.StreamWriter = func(w io.Writer) error {
		writer := NewWriter(w)
		return s.handler(writer)
	}

	return nil
}

// StatusCode returns 200 OK.
func (s SSEStreamResponse) StatusCode() int { return http.StatusOK }

// ResponseBody returns nil.
func (s SSEStreamResponse) ResponseBody() any { return nil }

// ResponseHeaders returns SSE headers.
func (s SSEStreamResponse) ResponseHeaders() http.Header {
	h := make(http.Header)
	h.Set(header.ContentType, "text/event-stream")
	h.Set(header.CacheControl, "no-cache")
	h.Set(header.Connection, "keep-alive")
	h.Set(header.XAccelBuffering, "no")

	return h
}

// ResponseCookies returns nil.
func (s SSEStreamResponse) ResponseCookies() []*http.Cookie { return nil }

// Stream initiates a Server-Sent Events stream.
func Stream(fn StreamHandler) SSEStreamResponse {
	return SSEStreamResponse{handler: fn}
}
