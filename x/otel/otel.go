// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package otel provides a zero-dependency, ultra-high-performance server-side OpenTelemetry (OTel)
// distributed tracing engine strictly conforming to W3C TraceContext and OTLP/HTTP specifications.
package otel

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

type spanContextKey struct{}

// Span represents an active OpenTelemetry distributed tracing server span.
type Span struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	TraceFlags   string
	Name         string
	StartTime    time.Time
	EndTime      time.Time
	Attributes   map[string]any
	Status       string
	StatusMsg    string
	mu           sync.Mutex
}

// SetAttribute sets a key-value attribute on the span.
func (s *Span) SetAttribute(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Attributes == nil {
		s.Attributes = make(map[string]any)
	}

	s.Attributes[key] = value
}

// SpanFromContext retrieves the current active OpenTelemetry Span from context.
func SpanFromContext(ctx context.Context) *Span {
	if ctx == nil {
		return nil
	}

	if span, ok := ctx.Value(spanContextKey{}).(*Span); ok {
		return span
	}

	return nil
}

// TraceIDFromContext retrieves the current TraceID string from context.
func TraceIDFromContext(ctx context.Context) string {
	if s := SpanFromContext(ctx); s != nil {
		return s.TraceID
	}

	return ""
}

// Exporter defines an OpenTelemetry span export target.
type Exporter interface {
	Export(spans []*Span)
	Shutdown(ctx context.Context) error
}

type otlpExporter struct {
	endpoint string
	client   *http.Client
	queue    chan *Span
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

func newOTLPExporter(endpoint string) *otlpExporter {
	ctx, cancel := context.WithCancel(context.Background())
	exp := &otlpExporter{
		endpoint: strings.TrimSuffix(endpoint, "/") + "/v1/traces",
		client:   &http.Client{Timeout: 5 * time.Second},
		queue:    make(chan *Span, 4096),
		ctx:      ctx,
		cancel:   cancel,
	}

	exp.wg.Add(1)
	go exp.worker()

	return exp
}

func (e *otlpExporter) Export(spans []*Span) {
	for _, s := range spans {
		select {
		case e.queue <- s:
		default:
			// Discard if queue full to prevent caller blocking
		}
	}
}

func (e *otlpExporter) worker() {
	defer e.wg.Done()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var batch []*Span

	sendBatch := func() {
		if len(batch) == 0 {
			return
		}

		_ = e.sendOTLP(batch)
		batch = batch[:0]
	}

	for {
		select {
		case <-e.ctx.Done():
			sendBatch()
			return
		case span := <-e.queue:
			batch = append(batch, span)
			if len(batch) >= 128 {
				sendBatch()
			}
		case <-ticker.C:
			sendBatch()
		}
	}
}

func (e *otlpExporter) sendOTLP(spans []*Span) error {
	type KeyValue struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	}

	type SpanPayload struct {
		TraceID           string     `json:"traceId"`
		SpanID            string     `json:"spanId"`
		ParentSpanID      string     `json:"parentSpanId,omitempty"`
		Name              string     `json:"name"`
		Kind              int        `json:"kind"` // 2 = SPAN_KIND_SERVER
		StartTimeUnixNano string     `json:"startTimeUnixNano"`
		EndTimeUnixNano   string     `json:"endTimeUnixNano"`
		Attributes        []KeyValue `json:"attributes,omitempty"`
	}

	type ScopeSpans struct {
		Spans []SpanPayload `json:"spans"`
	}

	type ResourceSpans struct {
		ScopeSpans []ScopeSpans `json:"scopeSpans"`
	}

	type ExportPayload struct {
		ResourceSpans []ResourceSpans `json:"resourceSpans"`
	}

	payloadSpans := make([]SpanPayload, 0, len(spans))
	for _, s := range spans {
		attrs := make([]KeyValue, 0, len(s.Attributes))
		for k, v := range s.Attributes {
			attrs = append(attrs, KeyValue{Key: k, Value: map[string]any{"stringValue": fmt.Sprint(v)}})
		}

		payloadSpans = append(payloadSpans, SpanPayload{
			TraceID:           s.TraceID,
			SpanID:            s.SpanID,
			ParentSpanID:      s.ParentSpanID,
			Name:              s.Name,
			Kind:              2,
			StartTimeUnixNano: strconv.FormatInt(s.StartTime.UnixNano(), 10),
			EndTimeUnixNano:   strconv.FormatInt(s.EndTime.UnixNano(), 10),
			Attributes:        attrs,
		})
	}

	body, err := json.Marshal(ExportPayload{
		ResourceSpans: []ResourceSpans{
			{ScopeSpans: []ScopeSpans{{Spans: payloadSpans}}},
		},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(e.ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set(header.ContentType, header.MIMEApplicationJSON)

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return nil
}

func (e *otlpExporter) Shutdown(ctx context.Context) error {
	e.cancel()
	e.wg.Wait()

	return nil
}

// Config configures the OpenTelemetry server middleware.
type Config struct {
	// ServiceName is the service identifier emitted in traces. Default is "sein-service".
	ServiceName string
	// Exporter is the span export destination.
	Exporter Exporter
	// Filter skips tracing when returning true.
	Filter func(req *sein.Request) bool
}

// Option configures OTel settings.
type Option func(*Config)

// WithServiceName sets the service name attribute.
func WithServiceName(name string) Option {
	return func(c *Config) {
		c.ServiceName = name
	}
}

// WithExporter sets a custom span exporter.
func WithExporter(exporter Exporter) Option {
	return func(c *Config) {
		c.Exporter = exporter
	}
}

// WithOTLPExporter configures an asynchronous OTLP/HTTP exporter endpoint.
func WithOTLPExporter(endpoint string) Option {
	return func(c *Config) {
		c.Exporter = newOTLPExporter(endpoint)
	}
}

// WithFilter sets a skip condition.
func WithFilter(fn func(req *sein.Request) bool) Option {
	return func(c *Config) {
		c.Filter = fn
	}
}

func generateID(byteLen int) string {
	b := make([]byte, byteLen)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

func parseTraceParent(headerVal string) (traceID, parentSpanID, flags string, ok bool) {
	parts := strings.Split(headerVal, "-")
	if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 {
		return "", "", "", false
	}

	return parts[1], parts[2], parts[3], true
}

// New creates an OpenTelemetry distributed tracing middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		ServiceName: "sein-service",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			if cfg.Filter != nil && cfg.Filter(req) {
				return next(req)
			}

			traceID, parentSpanID, flags, ok := parseTraceParent(req.Header("traceparent"))
			if !ok {
				traceID = generateID(16)
				flags = "01"
			}

			spanID := generateID(8)
			traceParentOut := "00-" + traceID + "-" + spanID + "-" + flags

			span := &Span{
				TraceID:      traceID,
				SpanID:       spanID,
				ParentSpanID: parentSpanID,
				TraceFlags:   flags,
				Name:         req.Method() + " " + req.Path(),
				StartTime:    time.Now(),
				Attributes: map[string]any{
					"service.name":             cfg.ServiceName,
					"http.request.method":      req.Method(),
					"http.route":               req.Path(),
					"client.address":           req.ClientIP(),
					"user_agent.original":      req.Header(header.UserAgent),
					"url.scheme":               req.Scheme(),
					"server.address":           req.Host(),
				},
			}

			ctx := context.WithValue(req.Context(), spanContextKey{}, span)
			req.SetContext(ctx)

			res, err := next(req)
			span.EndTime = time.Now()

			statusCode := http.StatusOK
			if err != nil {
				span.Status = "ERROR"
				span.StatusMsg = err.Error()
				if domainErr, ok := err.(sein.DomainError); ok {
					statusCode = domainErr.HTTPStatus()
				} else if httpErr, ok := sein.AsHTTPError(err); ok {
					statusCode = httpErr.HTTPStatus()
				} else {
					statusCode = http.StatusInternalServerError
				}
			} else {
				span.Status = "OK"
				if holder, ok := res.(sein.ResponseHolder); ok {
					if code := holder.StatusCode(); code != 0 {
						statusCode = code
					}
				}
			}

			span.SetAttribute("http.response.status_code", statusCode)

			if cfg.Exporter != nil {
				cfg.Exporter.Export([]*Span{span})
			}

			if holder, ok := res.(sein.ResponseHolder); ok {
				resp := sein.OK[any](holder.ResponseBody()).
					WithStatus(holder.StatusCode()).
					WithHeader("traceparent", traceParentOut).
					WithHeader("X-Trace-ID", traceID)

				for k, vv := range holder.ResponseHeaders() {
					for _, v := range vv {
						resp = resp.WithHeader(k, v)
					}
				}

				return resp, err
			}

			resp := sein.OK[any](res).
				WithHeader("traceparent", traceParentOut).
				WithHeader("X-Trace-ID", traceID)

			return resp, err
		}
	}
}
