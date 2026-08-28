// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package session provides high-performance, thread-safe HTTP session management
// supporting in-memory storage, flash messages, and secure cookie lifecycle binding.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/lemon4ksan/sein"
)

// DefaultCookieName is the standard session cookie identifier.
const DefaultCookieName = "session_id"

// Store defines the persistent storage engine for session data.
type Store interface {
	Get(id string) (map[string]any, error)
	Set(id string, data map[string]any, ttl time.Duration) error
	Delete(id string) error
}

type memoryEntry struct {
	data      map[string]any
	expiresAt time.Time
}

// MemoryStore provides a thread-safe in-memory session store.
type MemoryStore struct {
	mu      sync.RWMutex
	entries map[string]memoryEntry
}

// NewMemoryStore creates a new in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		entries: make(map[string]memoryEntry),
	}
}

// Get retrieves session data from memory.
func (m *MemoryStore) Get(id string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[id]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, nil
	}

	return maps.Clone(entry.data), nil
}

// Set stores session data in memory.
func (m *MemoryStore) Set(id string, data map[string]any, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries[id] = memoryEntry{
		data:      maps.Clone(data),
		expiresAt: time.Now().Add(ttl),
	}

	return nil
}

// Delete removes session data from memory.
func (m *MemoryStore) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.entries, id)

	return nil
}

// Session represents an active user session.
type Session struct {
	id           string
	data         map[string]any
	flashes      map[string]any
	freshFlashes map[string]any
	store        Store
	ttl          time.Duration
	destroyed    bool
	modified     bool
	mu           sync.RWMutex
}

// ID returns the unique session ID.
func (s *Session) ID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.id
}

// Get retrieves a value from the session.
func (s *Session) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.data[key]
}

// GetString retrieves a string value from the session.
func (s *Session) GetString(key string) string {
	val := s.Get(key)
	if s, ok := val.(string); ok {
		return s
	}

	return ""
}

// GetInt retrieves an integer value from the session.
func (s *Session) GetInt(key string) int {
	val := s.Get(key)
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}

	return 0
}

// Set saves a key-value pair in the session.
func (s *Session) Set(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		s.data = make(map[string]any)
	}

	s.data[key] = val
	s.modified = true
}

// Delete removes a key from the session.
func (s *Session) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, key)
	s.modified = true
}

// Flash sets or retrieves a flash message (available only until the next request).
func (s *Session) Flash(key string, val ...any) any {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(val) > 0 {
		if s.freshFlashes == nil {
			s.freshFlashes = make(map[string]any)
		}

		s.freshFlashes[key] = val[0]
		s.modified = true

		return val[0]
	}

	return s.flashes[key]
}

// Destroy destroys the session and invalidates the session ID.
func (s *Session) Destroy() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.destroyed = true
	s.data = make(map[string]any)
	s.flashes = make(map[string]any)
	s.freshFlashes = make(map[string]any)

	if s.store != nil && s.id != "" {
		return s.store.Delete(s.id)
	}

	return nil
}

// From retrieves the active Session from the request context.
func From(req *sein.Request) *Session {
	sess, _ := sein.Get[*Session](req)

	return sess
}

// Config configures the Session middleware.
type Config struct {
	// CookieName is the name of the session cookie. Default is "session_id".
	CookieName string
	// Expiration is the session TTL. Default is 24 hours.
	Expiration time.Duration
	// Store is the session storage engine. Default is MemoryStore.
	Store Store
	// CookiePath is the cookie Path attribute. Default is "/".
	CookiePath string
	// CookieDomain is the cookie Domain attribute.
	CookieDomain string
	// CookieSameSite is the SameSite mode. Default is http.SameSiteLaxMode.
	CookieSameSite http.SameSite
	// CookieSecure sets HTTPS-only cookies. Default is false.
	CookieSecure bool
	// CookieHTTPOnly sets HttpOnly flag. Default is true.
	CookieHTTPOnly bool
}

// Option configures Session settings.
type Option func(*Config)

// WithCookieName sets the cookie name.
func WithCookieName(name string) Option {
	return func(c *Config) {
		c.CookieName = name
	}
}

// WithExpiration sets the session TTL.
func WithExpiration(d time.Duration) Option {
	return func(c *Config) {
		c.Expiration = d
	}
}

// WithStore sets the storage backend.
func WithStore(store Store) Option {
	return func(c *Config) {
		c.Store = store
	}
}

// WithCookieSecure sets whether cookies are secure (HTTPS only).
func WithCookieSecure(secure bool) Option {
	return func(c *Config) {
		c.CookieSecure = secure
	}
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

// New creates a session management middleware.
func New(opts ...Option) sein.Middleware {
	cfg := Config{
		CookieName:     DefaultCookieName,
		Expiration:     24 * time.Hour,
		Store:          NewMemoryStore(),
		CookiePath:     "/",
		CookieSameSite: http.SameSiteLaxMode,
		CookieHTTPOnly: true,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			cookieVal, _ := req.Cookie(cfg.CookieName)
			sessionID := cookieVal
			isNew := false

			var (
				existingData map[string]any
				err          error
			)

			if sessionID != "" {
				existingData, err = cfg.Store.Get(sessionID)
				if err != nil || existingData == nil {
					existingData = make(map[string]any)
				}
			} else {
				sessionID = generateID()
				existingData = make(map[string]any)
				isNew = true
			}

			// Extract flashes
			flashes := make(map[string]any)
			if f, ok := existingData["_flashes"].(map[string]any); ok {
				maps.Copy(flashes, f)
				delete(existingData, "_flashes")
			}

			sess := &Session{
				id:           sessionID,
				data:         existingData,
				flashes:      flashes,
				freshFlashes: make(map[string]any),
				store:        cfg.Store,
				ttl:          cfg.Expiration,
			}

			sein.Set(req, sess)

			res, err := next(req)
			if err != nil {
				return nil, err
			}

			// Save session if modified or new
			if sess.destroyed {
				_ = cfg.Store.Delete(sessionID)
				// #nosec G124 -- Expired cookie to clear session
				cookie := &http.Cookie{
					Name:     cfg.CookieName,
					Value:    "",
					Path:     cfg.CookiePath,
					Domain:   cfg.CookieDomain,
					MaxAge:   -1,
					SameSite: cfg.CookieSameSite,
					Secure:   cfg.CookieSecure,
					HttpOnly: cfg.CookieHTTPOnly,
				}

				return attachSessionCookie(res, cookie), nil
			}

			if sess.modified || isNew || len(flashes) > 0 {
				dataToSave := maps.Clone(sess.data)
				if dataToSave == nil {
					dataToSave = make(map[string]any)
				}

				if len(sess.freshFlashes) > 0 {
					dataToSave["_flashes"] = sess.freshFlashes
				}

				_ = cfg.Store.Set(sessionID, dataToSave, cfg.Expiration)
			}

			// #nosec G124 -- Session cookie attributes are configurable with Lax and HttpOnly defaults
			cookie := &http.Cookie{
				Name:     cfg.CookieName,
				Value:    sessionID,
				Path:     cfg.CookiePath,
				Domain:   cfg.CookieDomain,
				MaxAge:   int(cfg.Expiration.Seconds()),
				SameSite: cfg.CookieSameSite,
				Secure:   cfg.CookieSecure,
				HttpOnly: cfg.CookieHTTPOnly,
			}

			return attachSessionCookie(res, cookie), nil
		}
	}
}

func attachSessionCookie(res any, cookie *http.Cookie) any {
	if holder, ok := res.(sein.ResponseHolder); ok {
		resp := sein.OK[any](holder.ResponseBody()).
			WithStatus(holder.StatusCode()).
			WithCookie(cookie)

		for k, vv := range holder.ResponseHeaders() {
			for _, v := range vv {
				resp = resp.WithHeader(k, v)
			}
		}

		return resp
	}

	return sein.OK[any](res).WithCookie(cookie)
}
