// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package qpack implements QPACK: Field Compression for HTTP/3 (RFC 9204).
package qpack

import "strings"

// HeaderField represents a single HTTP/3 header or trailer field.
type HeaderField struct {
	Name  string
	Value string
}

// IsPseudo reports whether the header field is an HTTP/3 pseudo-header (e.g. :method, :path).
func (hf HeaderField) IsPseudo() bool {
	return strings.HasPrefix(hf.Name, ":")
}

// Size returns the size of the header field in bytes as defined by RFC 9204 (name + value + 32 bytes overhead).
func (hf HeaderField) Size() int {
	return len(hf.Name) + len(hf.Value) + 32
}

// String formats the header field as "Name: Value".
func (hf HeaderField) String() string {
	return hf.Name + ": " + hf.Value
}
