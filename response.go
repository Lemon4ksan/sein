// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"net/http"
	"strings"
	"time"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/sein/internal/fast/h1engine"
)

// Responder is an interface that allows custom types to control their exact wire serialization for net/http.
type Responder interface {
	WriteResponse(w http.ResponseWriter) error
}

// DirectH1Responder is an interface for direct serialization to the native H1 response.
type DirectH1Responder interface {
	WriteToH1(res *h1engine.Response) error
}

// ResponseHolder allows middlewares to inspect response metadata and payload.
type ResponseHolder interface {
	StatusCode() int
	ResponseBody() any
	ResponseHeaders() http.Header
	ResponseCookies() []*http.Cookie
}

// Response is a type-safe HTTP response container carrying status, headers, and body.
type Response[T any] struct {
	Status  int
	Body    T
	Headers http.Header
	Cookies []*http.Cookie
}

// StatusCode returns the HTTP status code.
func (r Response[T]) StatusCode() int {
	return r.Status
}

// ResponseBody returns the generic body payload.
func (r Response[T]) ResponseBody() any {
	return r.Body
}

// ResponseHeaders returns the response headers map.
func (r Response[T]) ResponseHeaders() http.Header {
	return r.Headers
}

// ResponseCookies returns attached cookies.
func (r Response[T]) ResponseCookies() []*http.Cookie {
	return r.Cookies
}

// WriteToH1 serializes the response directly into an h1.Response with zero net/http allocations.
func (r Response[T]) WriteToH1(res *h1engine.Response) error {
	res.Headers.AddFromHTTP(r.Headers)
	res.Cookies = append(res.Cookies, r.Cookies...)

	status := generic.Coalesce(r.Status, http.StatusOK)
	res.StatusCode = status

	if status == http.StatusNoContent || status == http.StatusNotModified {
		return nil
	}

	switch v := any(r.Body).(type) {
	case nil:
		return nil
	case []byte:
		if res.Headers.Get(header.ContentType) == "" {
			res.Headers.Set(header.ContentType, header.MIMEApplicationOctetStream)
		}

		res.Body = append(res.Body, v...)

		return nil

	case string:
		if res.Headers.Get(header.ContentType) == "" {
			res.Headers.Set(header.ContentType, header.MIMETextPlainCharsetUTF8)
		}

		res.Body = append(res.Body, bytesconv.S2B(v)...)

		return nil

	default:
		if res.Headers.Get(header.ContentType) == "" {
			res.Headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)
		}

		data, err := json.Marshal(v)
		if err != nil {
			return err
		}

		res.Body = append(res.Body, data...)

		return nil
	}
}

// WriteResponse serializes the response to the given http.ResponseWriter.
func (r Response[T]) WriteResponse(w http.ResponseWriter) error {
	copyHTTPHeaders(w.Header(), r.Headers)

	for _, c := range r.Cookies {
		http.SetCookie(w, c)
	}

	status := generic.Coalesce(r.Status, http.StatusOK)

	// 204 No Content or 304 Not Modified
	if status == http.StatusNoContent || status == http.StatusNotModified {
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
		_, err := w.Write(bytesconv.S2B(v))

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

// WithHeaders merges all key-value pairs from headers into the response.
func (r Response[T]) WithHeaders(headers http.Header) Response[T] {
	if len(headers) == 0 {
		return r
	}

	if r.Headers == nil {
		r.Headers = make(http.Header, len(headers))
	}

	copyHTTPHeaders(r.Headers, headers)

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

// WithETag sets the ETag header with quotes automatically formatted if omitted.
func (r Response[T]) WithETag(etag string) Response[T] {
	if !strings.HasPrefix(etag, "\"") && !strings.HasPrefix(etag, "W/\"") {
		etag = "\"" + etag + "\""
	}

	return r.WithHeader(header.ETag, etag)
}

// WithLastModified sets the Last-Modified header formatted per RFC 7232.
func (r Response[T]) WithLastModified(t time.Time) Response[T] {
	return r.WithHeader(header.LastModified, t.UTC().Format(http.TimeFormat))
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

// NotModified creates a 304 Not Modified conditional response.
func NotModified() Response[any] {
	return Response[any]{
		Status: http.StatusNotModified,
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

// StatusWith creates a response with custom status, body, and headers.
func StatusWith[T any](status int, body T, headers http.Header) Response[T] {
	return Response[T]{
		Status:  status,
		Body:    body,
		Headers: headers,
	}
}

// HTML creates an HTML response (text/html; charset=utf-8).
func HTML(content string) Response[string] {
	r := Response[string]{
		Status: http.StatusOK,
		Body:   content,
	}

	return r.WithHeader(header.ContentType, "text/html; charset=utf-8")
}
