// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package html provides zero-allocation HTML component rendering and native HTMX integration for sein.
package html

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"

	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/sein"
)

var bufPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

// Component represents any renderable HTML template or component.
// It is 100% compatible with a-h/templ generated components without requiring third-party imports.
type Component interface {
	Render(ctx context.Context, w io.Writer) error
}

// Render executes the component and returns a typed HTML response.
func Render(ctx context.Context, c Component) (sein.Response[[]byte], error) {
	buf, ok := bufPool.Get().(*bytes.Buffer)
	if !ok {
		buf = bytes.NewBuffer(make([]byte, 0, 4096))
	}

	buf.Reset()
	defer bufPool.Put(buf)

	if err := c.Render(ctx, buf); err != nil {
		return sein.Response[[]byte]{}, err
	}

	// Copy bytes to response slice
	payload := make([]byte, buf.Len())
	copy(payload, buf.Bytes())

	return sein.OK(payload).
		WithHeader(header.ContentType, header.MIMETextHTMLCharsetUTF8), nil
}

// String wraps raw HTML markup into a typed sein.Response with text/html content type.
func String(htmlMarkup string) sein.Response[string] {
	return sein.OK(htmlMarkup).
		WithHeader(header.ContentType, header.MIMETextHTMLCharsetUTF8)
}

// Raw wraps raw HTML byte slice into a typed sein.Response with text/html content type.
func Raw(htmlBytes []byte) sein.Response[[]byte] {
	return sein.OK(htmlBytes).
		WithHeader(header.ContentType, header.MIMETextHTMLCharsetUTF8)
}

// HTMX Request Inspection Helpers

// IsHTMX reports whether the incoming request was issued by the HTMX engine.
func IsHTMX(req *sein.Request) bool {
	return req.Header("HX-Request") == "true"
}

// IsBoosted reports whether the request was made via an hx-boosted element.
func IsBoosted(req *sein.Request) bool {
	return req.Header("HX-Boosted") == "true"
}

// IsHistoryRestoreRequest reports whether the request is for history restoration after a miss in local history cache.
func IsHistoryRestoreRequest(req *sein.Request) bool {
	return req.Header("HX-History-Restore-Request") == "true"
}

// Target returns the ID of the target element (from HX-Target header).
func Target(req *sein.Request) string {
	return req.Header("HX-Target")
}

// CurrentURL returns the current URL of the browser (from HX-Current-URL header).
func CurrentURL(req *sein.Request) string {
	return req.Header("HX-Current-URL")
}

// Prompt returns the user response to an hx-prompt dialog.
func Prompt(req *sein.Request) string {
	return req.Header("HX-Prompt")
}

// TriggerName returns the name of the triggered element (from HX-Trigger-Name header).
func TriggerName(req *sein.Request) string {
	return req.Header("HX-Trigger-Name")
}

// TriggerID returns the ID of the triggered element (from HX-Trigger header).
func TriggerID(req *sein.Request) string {
	return req.Header("HX-Trigger")
}

// HTMX Response Modifiers

// Trigger triggers client-side events via the HX-Trigger header.
func Trigger[T any](res sein.Response[T], event string) sein.Response[T] {
	return res.WithHeader("HX-Trigger", event)
}

// TriggerAfterSettle triggers client-side events after the settling step.
func TriggerAfterSettle[T any](res sein.Response[T], event string) sein.Response[T] {
	return res.WithHeader("HX-Trigger-After-Settle", event)
}

// TriggerAfterSwap triggers client-side events after the swap step.
func TriggerAfterSwap[T any](res sein.Response[T], event string) sein.Response[T] {
	return res.WithHeader("HX-Trigger-After-Swap", event)
}

// Redirect performs a client-side redirect that does not do a full page reload.
func Redirect[T any](res sein.Response[T], url string) sein.Response[T] {
	return res.WithHeader("HX-Redirect", url)
}

// Location allows a client-side redirect that does not do a full page reload with path context.
func Location[T any](res sein.Response[T], url string) sein.Response[T] {
	return res.WithHeader("HX-Location", url)
}

// Refresh forces a client-side refresh of the current page.
func Refresh[T any](res sein.Response[T]) sein.Response[T] {
	return res.WithHeader("HX-Refresh", "true")
}

// PushURL pushes a new URL into the browser history stack.
func PushURL[T any](res sein.Response[T], url string) sein.Response[T] {
	return res.WithHeader("HX-Push-Url", url)
}

// ReplaceURL replaces the current URL in the browser location bar.
func ReplaceURL[T any](res sein.Response[T], url string) sein.Response[T] {
	return res.WithHeader("HX-Replace-Url", url)
}

// Reswap specifies how the response will be swapped (e.g. "innerHTML", "outerHTML", "beforebegin").
func Reswap[T any](res sein.Response[T], swapStrategy string) sein.Response[T] {
	return res.WithHeader("HX-Reswap", swapStrategy)
}

// Retarget specifies a CSS selector that updates the target of the content update.
func Retarget[T any](res sein.Response[T], cssSelector string) sein.Response[T] {
	return res.WithHeader("HX-Retarget", cssSelector)
}

// StopPolling sets HTTP 286 status code to inform HTMX polling triggers to stop.
func StopPolling[T any](res sein.Response[T]) sein.Response[T] {
	return res.WithStatus(286)
}

// PreventDefault returns HTTP 204 No Content to cancel default event behavior in HTMX.
func PreventDefault() sein.Response[any] {
	return sein.NoContent().WithStatus(http.StatusNoContent)
}
