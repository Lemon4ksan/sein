// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package metadata provides gRPC key-value metadata management for requests and responses.
package metadata

import (
	"context"
	"net/http"
	"slices"
	"strings"
)

// MD is a mapping from metadata keys to slices of corresponding values.
type MD map[string][]string

// Len returns the number of items in MD.
func (md MD) Len() int {
	return len(md)
}

// Copy returns a shallow copy of md.
func (md MD) Copy() MD {
	out := make(MD, len(md))
	for k, v := range md {
		out[k] = slices.Clone(v)
	}

	return out
}

// Get obtains the values for a given key.
func (md MD) Get(k string) []string {
	k = strings.ToLower(k)

	return md[k]
}

// Set sets the value of a given key with a slice of values.
func (md MD) Set(k string, vals ...string) {
	if len(vals) == 0 {
		return
	}
	k = strings.ToLower(k)
	md[k] = vals
}

// Append adds the values to key k, not overwriting what was already there.
func (md MD) Append(k string, vals ...string) {
	if len(vals) == 0 {
		return
	}
	k = strings.ToLower(k)
	md[k] = append(md[k], vals...)
}

// Delete removes the values for key.
func (md MD) Delete(k string) {
	delete(md, strings.ToLower(k))
}

// CopyToHTTP copies all metadata key-value pairs into the provided http.Header.
func (md MD) CopyToHTTP(dst http.Header) {
	for k, vals := range md {
		for _, val := range vals {
			dst.Add(k, val)
		}
	}
}

// New creates an MD from a given key-value map.
func New(m map[string]string) MD {
	md := make(MD, len(m))
	for k, val := range m {
		key := strings.ToLower(k)
		md[key] = []string{val}
	}

	return md
}

// Pairs returns an MD formed by the mapping of key, value ... pairs.
func Pairs(kv ...string) MD {
	if len(kv)%2 == 1 {
		panic("metadata.Pairs got an odd number of input pairs")
	}
	md := make(MD, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		key := strings.ToLower(kv[i])
		md[key] = append(md[key], kv[i+1])
	}

	return md
}

type incomingKey struct{}
type outgoingKey struct{}
type headerKey struct{}

// NewIncomingContext creates a new context with incoming MD attached.
func NewIncomingContext(ctx context.Context, md MD) context.Context {
	return context.WithValue(ctx, incomingKey{}, md)
}

// FromIncomingContext returns the incoming metadata in ctx if it exists.
func FromIncomingContext(ctx context.Context) (MD, bool) {
	md, ok := ctx.Value(incomingKey{}).(MD)

	return md, ok
}

// NewOutgoingContext creates a new context with outgoing MD attached.
func NewOutgoingContext(ctx context.Context, md MD) context.Context {
	return context.WithValue(ctx, outgoingKey{}, md)
}

// FromOutgoingContext returns the outgoing metadata in ctx if it exists.
func FromOutgoingContext(ctx context.Context) (MD, bool) {
	md, ok := ctx.Value(outgoingKey{}).(MD)

	return md, ok
}

// ServerMetadataContext holds headers and trailers for in-flight requests.
type ServerMetadataContext struct {
	Header  MD
	Trailer MD
}

// NewServerMetadataContext returns a context configured with mutable Header and Trailer slots.
func NewServerMetadataContext(ctx context.Context) (context.Context, *ServerMetadataContext) {
	sm := &ServerMetadataContext{
		Header:  make(MD),
		Trailer: make(MD),
	}

	ctx = context.WithValue(ctx, headerKey{}, sm)

	return ctx, sm
}

// SetHeader sets the header metadata to be sent from the server.
func SetHeader(ctx context.Context, md MD) error {
	sm, ok := ctx.Value(headerKey{}).(*ServerMetadataContext)
	if !ok {
		return nil
	}
	for k, v := range md {
		sm.Header.Append(k, v...)
	}

	return nil
}

// SetTrailer sets the trailer metadata to be sent from the server.
func SetTrailer(ctx context.Context, md MD) error {
	sm, ok := ctx.Value(headerKey{}).(*ServerMetadataContext)
	if !ok {
		return nil
	}
	for k, v := range md {
		sm.Trailer.Append(k, v...)
	}

	return nil
}
