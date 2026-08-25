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

type mockUserController struct {
	db map[uint64]string
}

func (c *mockUserController) Mount(g *sein.Group) {
	g.GETReq("/:id", c.get)
	g.POST("", c.create)
}

func (c *mockUserController) get(req *sein.Request) (UserResponse, error) {
	id := req.Param("id").AsUint64()
	name, ok := c.db[id]
	if !ok {
		return UserResponse{}, sein.ErrNotFound("user not found")
	}
	return UserResponse{ID: id, Name: name}, nil
}

func (c *mockUserController) create(ctx context.Context, req CreateUserDTO) (UserResponse, error) {
	c.db[77] = req.Name
	return UserResponse{ID: 77, Name: req.Name, Email: req.Email}, nil
}

func TestControllerMountAndGrouping(t *testing.T) {
	app := sein.New()

	ctrl := &mockUserController{
		db: map[uint64]string{10: "Charlie"},
	}

	api := app.Group("/api/v1")
	ctrl.Mount(api.Group("/users"))

	// 1. Test GET /api/v1/users/10
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/users/10", nil)
	recGet := httptest.NewRecorder()
	app.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recGet.Code)
	}

	var u UserResponse
	_ = json.Unmarshal(recGet.Body.Bytes(), &u)
	if u.ID != 10 || u.Name != "Charlie" {
		t.Fatalf("unexpected user: %+v", u)
	}

	// 2. Test POST /api/v1/users
	body, _ := json.Marshal(CreateUserDTO{Name: "David", Email: "david@test.com"})
	reqPost := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	recPost := httptest.NewRecorder()
	app.ServeHTTP(recPost, reqPost)

	if recPost.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recPost.Code)
	}

	if ctrl.db[77] != "David" {
		t.Fatalf("expected user 77 to be David in db, got %s", ctrl.db[77])
	}
}

var (
	ErrUserEmailBusy    = sein.DefineError(409, "EMAIL_ALREADY_EXISTS", "Email address is already in use")
	ErrAccountSuspended = sein.DefineError(403, "ACCOUNT_SUSPENDED", "Account has been suspended")
)

func TestDomainErrors(t *testing.T) {
	app := sein.New()

	app.POST("/api/v1/register", func(ctx context.Context, req CreateUserDTO) (UserResponse, error) {
		if req.Email == "taken@example.com" {
			// Zero manual strings, zero manual status codes
			return UserResponse{}, ErrUserEmailBusy
		}
		if req.Email == "banned@example.com" {
			return UserResponse{}, ErrAccountSuspended.WithDetail("ban_reason", "rule violation")
		}
		return UserResponse{ID: 1, Name: req.Name, Email: req.Email}, nil
	})

	// 1. Test Conflict error (409)
	body, _ := json.Marshal(CreateUserDTO{Name: "Evil", Email: "taken@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", rec.Code)
	}

	var errPayload struct {
		Status  int    `json:"status"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &errPayload)

	if errPayload.Status != 409 || errPayload.Code != "EMAIL_ALREADY_EXISTS" {
		t.Fatalf("unexpected error payload: %+v", errPayload)
	}

	// 2. Test Forbidden error with details (403)
	bodyBanned, _ := json.Marshal(CreateUserDTO{Name: "Banned", Email: "banned@example.com"})
	reqBanned := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(bodyBanned))
	recBanned := httptest.NewRecorder()
	app.ServeHTTP(recBanned, reqBanned)

	if recBanned.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", recBanned.Code)
	}

	var bannedPayload struct {
		Status  int            `json:"status"`
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	}
	_ = json.Unmarshal(recBanned.Body.Bytes(), &bannedPayload)

	if bannedPayload.Code != "ACCOUNT_SUSPENDED" || bannedPayload.Details["ban_reason"] != "rule violation" {
		t.Fatalf("unexpected banned payload: %+v", bannedPayload)
	}
}

type TestSnowflake uint64

var (
	ParamTestID   = sein.PathParam[TestSnowflake]("id")
	QueryTestPage = sein.QueryParam[int]("page")
	HeaderTestKey = sein.HeaderParam[string]("X-Api-Key")
)

func TestTypedParamDescriptors(t *testing.T) {
	app := sein.New()

	app.GETReq("/items/:id", func(req *sein.Request) (map[string]any, error) {
		id, err := ParamTestID.Get(req)
		if err != nil {
			return nil, err
		}

		page := QueryTestPage.GetOr(req, 1)
		apiKey, err := HeaderTestKey.Get(req)
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"id":      id,
			"page":    page,
			"api_key": apiKey,
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/items/8888?page=5", nil)
	req.Header.Set("X-Api-Key", "secret-token")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &res)

	if res["id"].(float64) != 8888 || res["page"].(float64) != 5 || res["api_key"] != "secret-token" {
		t.Fatalf("unexpected response: %+v", res)
	}
}



