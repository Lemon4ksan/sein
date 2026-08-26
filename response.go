// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"encoding/json"
	"net/http"

	"github.com/lemon4ksan/foundation/net/http/header"
)

// Responder is an interface that allows custom types to control their exact wire serialization.
type Responder interface {
	WriteResponse(w http.ResponseWriter) error
}

// Response is a type-safe HTTP response container carrying status, headers, and body.
type Response[T any] struct {
	Status  int
	Body    T
	Headers http.Header
	Cookies []*http.Cookie
}

// WriteResponse serializes the response to the given http.ResponseWriter.
func (r Response[T]) WriteResponse(w http.ResponseWriter) error {
	// Write headers
	for k, vv := range r.Headers {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	// Write cookies
	for _, c := range r.Cookies {
		http.SetCookie(w, c)
	}

	status := r.Status
	if status == 0 {
		status = http.StatusOK
	}

	// 204 No Content
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return nil
	}

	// If body is already byte slice or string, write directly
	switch v := any(r.Body).(type) {
	case nil:
		w.WriteHeader(status)
		return nil
	case []byte:
		if w.Header().Get(header.ContentType) == "" {
			w.Header().Set(header.ContentType, header.MIMEApplicationOctetStream)
		}
		w.WriteHeader(status)
		_, err := w.Write(v)
		return err
	case string:
		if w.Header().Get(header.ContentType) == "" {
			w.Header().Set(header.ContentType, header.MIMETextPlainCharsetUTF8)
		}
		w.WriteHeader(status)
		_, err := w.Write([]byte(v))
		return err
	default:
		if w.Header().Get(header.ContentType) == "" {
			w.Header().Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)
		}
		w.WriteHeader(status)
		return json.NewEncoder(w).Encode(v)
	}
}

// WithHeader adds a response header.
func (r Response[T]) WithHeader(key, value string) Response[T] {
	if r.Headers == nil {
		r.Headers = make(http.Header)
	}
	r.Headers.Set(key, value)
	return r
}

// WithCookie attaches a set-cookie instruction to the response.
func (r Response[T]) WithCookie(c *http.Cookie) Response[T] {
	r.Cookies = append(r.Cookies, c)
	return r
}

// WithStatus changes the HTTP status code.
func (r Response[T]) WithStatus(code int) Response[T] {
	r.Status = code
	return r
}

// OK creates a 200 OK response with the given body.
func OK[T any](body T) Response[T] {
	return Response[T]{
		Status: http.StatusOK,
		Body:   body,
	}
}

// Created creates a 201 Created response with the given body.
func Created[T any](body T) Response[T] {
	return Response[T]{
		Status: http.StatusCreated,
		Body:   body,
	}
}

// Accepted creates a 202 Accepted response with the given body.
func Accepted[T any](body T) Response[T] {
	return Response[T]{
		Status: http.StatusAccepted,
		Body:   body,
	}
}

// NoContent creates a 204 No Content response.
func NoContent() Response[any] {
	return Response[any]{
		Status: http.StatusNoContent,
	}
}

// Redirect creates a 302/307 redirect response.
func Redirect(targetURL string, status ...int) Response[any] {
	code := http.StatusFound
	if len(status) > 0 {
		code = status[0]
	}
	r := Response[any]{Status: code}
	return r.WithHeader(header.Location, targetURL)
}
