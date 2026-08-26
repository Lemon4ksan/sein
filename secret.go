// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"encoding/json"
	"fmt"
)

// Secret wraps sensitive data (passwords, tokens, API keys) to prevent accidental leakage in logs and JSON outputs.
type Secret[T any] struct {
	val T
}

// NewSecret creates a new protected [Secret] wrapping val.
func NewSecret[T any](val T) Secret[T] {
	return Secret[T]{val: val}
}

// Value returns the raw sensitive value for authorized business logic.
func (s Secret[T]) Value() T {
	return s.val
}

// Expose returns the raw sensitive value. Synonym for [Value].
func (s Secret[T]) Expose() T {
	return s.val
}

// String masks the secret when formatted with %s, %v, or fmt.Println.
func (s Secret[T]) String() string {
	return "******"
}

// GoString masks the secret when formatted with %#v in debug prints.
func (s Secret[T]) GoString() string {
	return "sein.Secret(******)"
}

// Format masks the secret during fmt.Sprintf printing.
func (s Secret[T]) Format(f fmt.State, verb rune) {
	_, _ = f.Write([]byte("******"))
}

// MarshalJSON safely serializes the secret as a masked string to prevent leakage in API responses.
func (s Secret[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal("******")
}

// UnmarshalJSON deserializes the raw value into the protected container.
func (s *Secret[T]) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &s.val)
}
