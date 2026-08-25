// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lemon4ksan/sein"
)

type CreateUserDTO struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type UserResponse struct {
	ID    uint64 `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type UserSession struct {
	UserID uint64
	Role   string
}

func TestPureHandler(t *testing.T) {
	app := sein.New()

	// Pure Handler: (context.Context, Req) -> (Res, error)
	// No HTTP framework objects in business logic!
	createUser := func(ctx context.Context, req CreateUserDTO) (UserResponse, error) {
		if req.Name == "" {
			return UserResponse{}, sein.ErrBadRequest("name cannot be empty")
		}
		return UserResponse{
			ID:    42,
			Name:  req.Name,
			Email: req.Email,
		}, nil
	}

	sein.POST(app, "/api/v1/users", createUser)

	// Test 1: Successful creation
	body, _ := json.Marshal(CreateUserDTO{Name: "Alice", Email: "alice@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp UserResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID != 42 || resp.Name != "Alice" {
		t.Fatalf("unexpected user response: %+v", resp)
	}

	// Test 2: Validation error -> 400 Bad Request
	badBody, _ := json.Marshal(CreateUserDTO{Name: ""})
	reqBad := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(badBody))
	recBad := httptest.NewRecorder()

	app.ServeHTTP(recBad, reqBad)

	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recBad.Code)
	}
}

func TestRequestParamsAndTypedContext(t *testing.T) {
	app := sein.New()

	// Auth Middleware storing typed UserSession into Request
	authMiddleware := func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			token, ok := req.BearerToken()
			if !ok || token != "secret-token" {
				return nil, sein.ErrUnauthorized("missing or invalid bearer token")
			}
			// Store typed session (Zero string keys, zero type assertions)
			sein.Set(req, UserSession{UserID: 100, Role: "admin"})
			return next(req)
		}
	}

	// Handler with Request view
	sein.GETReq(app, "/api/v1/users/:id", func(req *sein.Request) (sein.Response[UserResponse], error) {
		session, ok := sein.Get[UserSession](req)
		if !ok {
			return sein.Response[UserResponse]{}, sein.ErrUnauthorized("session missing")
		}

		id := req.Param("id").AsUint64()
		if id == 0 {
			return sein.Response[UserResponse]{}, sein.ErrBadRequest("invalid user id")
		}

		if id == 999 {
			return sein.Response[UserResponse]{}, sein.ErrNotFound("user not found")
		}

		user := UserResponse{
			ID:    id,
			Name:  "Bob (by " + session.Role + ")",
			Email: "bob@example.com",
		}

		return sein.Created(user).WithHeader("X-Handled-By", "sein"), nil
	}, authMiddleware)

	// 1. Unauthorized request
	reqUnauth := httptest.NewRequest(http.MethodGet, "/api/v1/users/42", nil)
	recUnauth := httptest.NewRecorder()
	app.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recUnauth.Code)
	}

	// 2. Authorized request
	reqAuth := httptest.NewRequest(http.MethodGet, "/api/v1/users/42", nil)
	reqAuth.Header.Set("Authorization", "Bearer secret-token")
	recAuth := httptest.NewRecorder()
	app.ServeHTTP(recAuth, reqAuth)

	if recAuth.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", recAuth.Code)
	}
	if recAuth.Header().Get("X-Handled-By") != "sein" {
		t.Fatalf("expected X-Handled-By header, got %s", recAuth.Header().Get("X-Handled-By"))
	}

	// 3. Not Found
	reqNotFound := httptest.NewRequest(http.MethodGet, "/api/v1/users/999", nil)
	reqNotFound.Header.Set("Authorization", "Bearer secret-token")
	recNotFound := httptest.NewRecorder()
	app.ServeHTTP(recNotFound, reqNotFound)
	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recNotFound.Code)
	}
}

func TestPanicRecovery(t *testing.T) {
	app := sein.New()
	app.Use(sein.Recovery())

	sein.GET(app, "/panic", func(ctx context.Context) (string, error) {
		panic("unexpected explosion!")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on panic, got %d", rec.Code)
	}
}
