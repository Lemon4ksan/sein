// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sentry provides lightweight, zero-dependency Sentry error monitoring and reporting middleware.
package sentry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein"
)

// Transport defines the delivery mechanism for Sentry events.
type Transport interface {
	Send(event *Event)
	Flush(ctx context.Context) error
}

// Event represents a Sentry error event.
type Event struct {
	EventID     string            `json:"event_id"`
	Timestamp   string            `json:"timestamp"`
	Platform    string            `json:"platform"`
	Level       string            `json:"level"`
	Message     string            `json:"message"`
	StackTrace  string            `json:"stacktrace,omitempty"`
	RequestInfo map[string]any    `json:"request,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

type httpTransport struct {
	apiURL  string
	authHdr string
	client  *http.Client
	queue   chan *Event
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

func newHTTPTransport(dsnStr string) (*httpTransport, error) {
	u, err := url.Parse(dsnStr)
	if err != nil || u.User == nil || len(u.Path) < 2 {
		return nil, fmt.Errorf("sentry: invalid DSN format: %s", dsnStr)
	}

	publicKey := u.User.Username()
	projectID := strings.TrimPrefix(u.Path, "/")
	apiURL := fmt.Sprintf("%s://%s/api/%s/store/", u.Scheme, u.Host, projectID)
	authHdr := fmt.Sprintf("Sentry sentry_version=7, sentry_client=sein/1.0, sentry_key=%s", publicKey)

	ctx, cancel := context.WithCancel(context.Background())
	t := &httpTransport{
		apiURL:  apiURL,
		authHdr: authHdr,
		client:  &http.Client{Timeout: 5 * time.Second},
		queue:   make(chan *Event, 2048),
		ctx:     ctx,
		cancel:  cancel,
	}

	t.wg.Add(1)
	go t.worker()

	return t, nil
}

func (t *httpTransport) Send(event *Event) {
	select {
	case t.queue <- event:
	default:
		// Drop on saturated queue
	}
}

func (t *httpTransport) worker() {
	defer t.wg.Done()

	for {
		select {
		case <-t.ctx.Done():
			return
		case event := <-t.queue:
			_ = t.postEvent(event)
		}
	}
}

func (t *httpTransport) postEvent(event *Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(t.ctx, http.MethodPost, t.apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set(header.ContentType, header.MIMEApplicationJSON)
	req.Header.Set("X-Sentry-Auth", t.authHdr)

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	return nil
}

func (t *httpTransport) Flush(ctx context.Context) error {
	t.cancel()
	t.wg.Wait()

	return nil
}

// Config configures the Sentry middleware.
type Config struct {
	// DSN is the Sentry project DSN string.
	DSN string
	// Environment is the deployment environment tag (e.g. "production", "staging"). Default is "production".
	Environment string
	// Release is the application release version.
	Release string
	// Transport overrides the default HTTP transport.
	Transport Transport
}

// Option configures Sentry settings.
type Option func(*Config)

// WithDSN sets the Sentry DSN.
func WithDSN(dsn string) Option {
	return func(c *Config) {
		c.DSN = dsn
	}
}

// WithEnvironment sets the environment tag.
func WithEnvironment(env string) Option {
	return func(c *Config) {
		c.Environment = env
	}
}

// WithRelease sets the release version tag.
func WithRelease(rel string) Option {
	return func(c *Config) {
		c.Release = rel
	}
}

// WithTransport sets a custom transport implementation.
func WithTransport(t Transport) Option {
	return func(c *Config) {
		c.Transport = t
	}
}

func generateEventID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

// New creates a Sentry error monitoring middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		Environment: "production",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.Transport == nil && cfg.DSN != "" {
		if t, err := newHTTPTransport(cfg.DSN); err == nil {
			cfg.Transport = t
		}
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (res any, err error) {
			defer func() {
				if r := recover(); r != nil {
					if cfg.Transport != nil {
						cfg.Transport.Send(&Event{
							EventID:    generateEventID(),
							Timestamp:  time.Now().UTC().Format(time.RFC3339),
							Platform:   "go",
							Level:      "fatal",
							Message:    fmt.Sprintf("panic: %v", r),
							StackTrace: string(debug.Stack()),
							RequestInfo: map[string]any{
								"url":    req.Path(),
								"method": req.Method(),
								"ip":     req.ClientIP(),
							},
							Tags: map[string]string{
								"environment": cfg.Environment,
								"release":     cfg.Release,
							},
						})
					}

					err = sein.ErrInternal("an unexpected panic occurred")
				}
			}()

			res, err = next(req)
			if err != nil && cfg.Transport != nil {
				cfg.Transport.Send(&Event{
					EventID:   generateEventID(),
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Platform:  "go",
					Level:     "error",
					Message:   err.Error(),
					RequestInfo: map[string]any{
						"url":    req.Path(),
						"method": req.Method(),
						"ip":     req.ClientIP(),
					},
					Tags: map[string]string{
						"environment": cfg.Environment,
						"release":     cfg.Release,
					},
				})
			}

			return res, err
		}
	}
}
