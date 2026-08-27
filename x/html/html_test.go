// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package html_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/x/html"
)

// mockTemplComponent implements html.Component (same signature as a-h/templ)
type mockTemplComponent struct {
	Title   string
	Message string
}

func (m mockTemplComponent) Render(_ context.Context, w io.Writer) error {
	_, err := fmt.Fprintf(w, "<div class=\"card\"><h1>%s</h1><p>%s</p></div>", m.Title, m.Message)
	return err
}

func TestHTML_RenderComponent(t *testing.T) {
	app := sein.New()

	app.Get("/view", func(ctx context.Context) (sein.Response[[]byte], error) {
		comp := mockTemplComponent{
			Title:   "Dashboard",
			Message: "Welcome back!",
		}
		return html.Render(ctx, comp)
	})

	req := httptest.NewRequest(http.MethodGet, "/view", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html; charset=utf-8, got %q", ct)
	}

	expectedBody := "<div class=\"card\"><h1>Dashboard</h1><p>Welcome back!</p></div>"
	if rec.Body.String() != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, rec.Body.String())
	}
}

func TestHTML_StringAndRaw(t *testing.T) {
	app := sein.New()

	app.Get("/raw", func(ctx context.Context) (sein.Response[string], error) {
		return html.String("<span>Pure HTML</span>"), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/raw", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if rec.Body.String() != "<span>Pure HTML</span>" {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestHTMX_HeadersAndModifiers(t *testing.T) {
	app := sein.New()

	app.Get("/htmx", func(ctx context.Context) (sein.Response[string], error) {
		res := html.String("<p>Updated</p>")
		res = html.Trigger(res, "itemUpdated")
		res = html.Reswap(res, "outerHTML")
		res = html.Retarget(res, "#target-container")
		res = html.PushURL(res, "/items/42")
		return res, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/htmx", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "main-content")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Header().Get("HX-Trigger") != "itemUpdated" {
		t.Errorf("expected HX-Trigger=itemUpdated, got %s", rec.Header().Get("HX-Trigger"))
	}
	if rec.Header().Get("HX-Reswap") != "outerHTML" {
		t.Errorf("expected HX-Reswap=outerHTML, got %s", rec.Header().Get("HX-Reswap"))
	}
	if rec.Header().Get("HX-Retarget") != "#target-container" {
		t.Errorf("expected HX-Retarget=#target-container, got %s", rec.Header().Get("HX-Retarget"))
	}
	if rec.Header().Get("HX-Push-Url") != "/items/42" {
		t.Errorf("expected HX-Push-Url=/items/42, got %s", rec.Header().Get("HX-Push-Url"))
	}
}
