// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h1

import (
	"strings"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"
	"github.com/lemon4ksan/foundation/silicon/simd"
)

// HeaderEntry represents a parsed HTTP header key-value pair.
type HeaderEntry struct {
	Key   string
	Value string
}

// Headers is a high-performance, slice-backed HTTP header storage.
type Headers struct {
	entries []HeaderEntry
}

// NewHeadersWithCapacity creates a Headers container with pre-allocated capacity.
func NewHeadersWithCapacity(cap int) Headers {
	return Headers{
		entries: make([]HeaderEntry, 0, cap),
	}
}

// Reset clears the headers slice while preserving underlying capacity.
func (h *Headers) Reset() {
	h.entries = h.entries[:0]
}

// Set sets the header key to value, replacing any existing entry.
func (h *Headers) Set(key, val string) {
	for i := range h.entries {
		if bytesconv.EqualFoldASCII(h.entries[i].Key, key) {
			h.entries[i].Value = val
			return
		}
	}

	h.entries = append(h.entries, HeaderEntry{Key: key, Value: val})
}

// Add appends a header key-value pair.
func (h *Headers) Add(key, val string) {
	h.entries = append(h.entries, HeaderEntry{Key: key, Value: val})
}

// Get retrieves the first value associated with key (case-insensitive ASCII).
func (h *Headers) Get(key string) string {
	for i := range h.entries {
		if bytesconv.EqualFoldASCII(h.entries[i].Key, key) {
			return h.entries[i].Value
		}
	}

	return ""
}

// Has reports whether key is present in headers (case-insensitive ASCII).
func (h *Headers) Has(key string) bool {
	for i := range h.entries {
		if bytesconv.EqualFoldASCII(h.entries[i].Key, key) {
			return true
		}
	}

	return false
}

// Del deletes all entries matching key.
func (h *Headers) Del(key string) {
	n := 0
	for _, entry := range h.entries {
		if !bytesconv.EqualFoldASCII(entry.Key, key) {
			h.entries[n] = entry
			n++
		}
	}

	h.entries = h.entries[:n]
}

// Entries returns the underlying slice of HeaderEntry.
func (h *Headers) Entries() []HeaderEntry {
	return h.entries
}

// ParseHeaderLine parses a single raw "Key: Value\r\n" or "Key: Value" byte slice using SIMD.
func (h *Headers) ParseHeaderLine(line []byte) bool {
	colonIdx := simd.ScanByteVector(line, ':')
	if colonIdx <= 0 {
		return false
	}

	key := strings.TrimSpace(bytesconv.B2S(line[:colonIdx]))
	val := strings.TrimSpace(bytesconv.B2S(line[colonIdx+1:]))

	h.entries = append(h.entries, HeaderEntry{Key: key, Value: val})

	return true
}

// IsKeepAlive returns true if the connection should remain open (HTTP/1.1 default unless Connection: close).
func (h *Headers) IsKeepAlive(proto string) bool {
	connHeader := h.Get(header.Connection)
	if proto == "HTTP/1.0" {
		return bytesconv.EqualFoldASCII(connHeader, header.ValueKeepAlive)
	}

	// HTTP/1.1 is keep-alive by default unless explicitly closed
	return !bytesconv.EqualFoldASCII(connHeader, header.ValueClose)
}
