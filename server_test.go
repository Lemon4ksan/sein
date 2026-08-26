// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"github.com/lemon4ksan/foundation/types/uuid"

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

	app.Post("/api/v1/users", createUser)

	// 1. Successful creation
	body, err := json.Marshal(CreateUserDTO{Name: "Alice", Email: "alice@example.com"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UserResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, uint64(42), resp.ID)
	assert.Equal(t, "Alice", resp.Name)

	// 2. Validation error -> 400 Bad Request
	badBody, err := json.Marshal(CreateUserDTO{Name: ""})
	require.NoError(t, err)

	reqBad := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(badBody))
	recBad := httptest.NewRecorder()
	app.ServeHTTP(recBad, reqBad)

	assert.Equal(t, http.StatusBadRequest, recBad.Code)
}

func TestRequestParamsAndTypedContext(t *testing.T) {
	app := sein.New()

	authMiddleware := func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			token, ok := req.BearerToken()
			if !ok || token != "secret-token" {
				return nil, sein.ErrUnauthorized("missing or invalid bearer token")
			}
			sein.Set(req, UserSession{UserID: 100, Role: "admin"})
			return next(req)
		}
	}

	app.Use(authMiddleware)

	type GetUserDTO struct {
		ID uint64 `path:"id"`
	}

	app.GetWith("/users/:id", func(ctx context.Context, req GetUserDTO) (UserResponse, error) {
		return UserResponse{
			ID:    req.ID,
			Name:  "Admin User",
			Email: "admin@example.com",
		}, nil
	})

	// 1. Unauthorized request (no bearer token)
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/users/100", nil)
	recNoAuth := httptest.NewRecorder()
	app.ServeHTTP(recNoAuth, reqNoAuth)
	assert.Equal(t, http.StatusUnauthorized, recNoAuth.Code)

	// 2. Authorized request
	reqAuth := httptest.NewRequest(http.MethodGet, "/users/100", nil)
	reqAuth.Header.Set("Authorization", "Bearer secret-token")
	recAuth := httptest.NewRecorder()
	app.ServeHTTP(recAuth, reqAuth)

	assert.Equal(t, http.StatusOK, recAuth.Code)
	var resp UserResponse
	require.NoError(t, json.Unmarshal(recAuth.Body.Bytes(), &resp))
	assert.Equal(t, uint64(100), resp.ID)
}

func TestPanicRecovery(t *testing.T) {
	app := sein.New()
	app.Use(sein.Recovery())

	app.Get("/panic", func(ctx context.Context) (string, error) {
		panic("unexpected explosion!")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

type mockUserController struct {
	db map[uint64]string
}

func (c *mockUserController) Mount(g *sein.Group) {
	type GetByIDDTO struct {
		ID uint64 `path:"id"`
	}
	g.GetWith("/:id", c.getByID)
	g.Post("", c.create)
}

func (c *mockUserController) getByID(ctx context.Context, req struct {
	ID uint64 `path:"id"`
}) (UserResponse, error) {
	name, exists := c.db[req.ID]
	if !exists {
		return UserResponse{}, sein.ErrNotFound("user not found")
	}
	return UserResponse{ID: req.ID, Name: name}, nil
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

	// 1. GET /api/v1/users/10
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/users/10", nil)
	recGet := httptest.NewRecorder()
	app.ServeHTTP(recGet, reqGet)

	assert.Equal(t, http.StatusOK, recGet.Code)
	var u UserResponse
	require.NoError(t, json.Unmarshal(recGet.Body.Bytes(), &u))
	assert.Equal(t, uint64(10), u.ID)
	assert.Equal(t, "Charlie", u.Name)

	// 2. POST /api/v1/users
	body, err := json.Marshal(CreateUserDTO{Name: "David", Email: "david@test.com"})
	require.NoError(t, err)

	reqPost := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	recPost := httptest.NewRecorder()
	app.ServeHTTP(recPost, reqPost)

	assert.Equal(t, http.StatusOK, recPost.Code)
	assert.Equal(t, "David", ctrl.db[77])
}

var (
	ErrUserEmailBusy    = sein.Conflict("EMAIL_ALREADY_EXISTS", "Email address is already in use")
	ErrAccountSuspended = sein.Forbidden("ACCOUNT_SUSPENDED", "Account has been suspended")
)

func TestDomainErrors(t *testing.T) {
	app := sein.New()

	app.Post("/api/v1/register", func(ctx context.Context, req CreateUserDTO) (UserResponse, error) {
		if req.Email == "taken@example.com" {
			return UserResponse{}, ErrUserEmailBusy
		}
		if req.Email == "banned@example.com" {
			return UserResponse{}, ErrAccountSuspended.WithDetail("ban_reason", "rule violation")
		}
		return UserResponse{ID: 1, Name: req.Name, Email: req.Email}, nil
	})

	// 1. 409 Conflict
	body, err := json.Marshal(CreateUserDTO{Name: "Evil", Email: "taken@example.com"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var errPayload struct {
		Status  int    `json:"status"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errPayload))
	assert.Equal(t, 409, errPayload.Status)
	assert.Equal(t, "EMAIL_ALREADY_EXISTS", errPayload.Code)

	// 2. 403 Forbidden
	bodyBanned, err := json.Marshal(CreateUserDTO{Name: "Banned", Email: "banned@example.com"})
	require.NoError(t, err)

	reqBanned := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(bodyBanned))
	recBanned := httptest.NewRecorder()
	app.ServeHTTP(recBanned, reqBanned)

	assert.Equal(t, http.StatusForbidden, recBanned.Code)
}

func TestTypedParamDescriptors(t *testing.T) {
	pUint := sein.ParamValue("18446744073709551615")
	assert.Equal(t, uint64(18446744073709551615), pUint.AsUint64())

	pInt := sein.ParamValue("-9223372036854775808")
	assert.Equal(t, int64(-9223372036854775808), pInt.AsInt64())

	pBool := sein.ParamValue("true")
	assert.True(t, pBool.AsBool())

	pEmpty := sein.ParamValue("")
	assert.Equal(t, uint64(0), pEmpty.AsUint64())
	assert.Equal(t, int(0), pEmpty.AsInt())
	assert.False(t, pEmpty.AsBool())
}

type SelfValidatingDTO struct {
	Age int `json:"age"`
}

func (dto SelfValidatingDTO) Validate() error {
	if dto.Age < 18 {
		return errors.New("must be at least 18 years old")
	}
	return nil
}

func TestValidatableDTO(t *testing.T) {
	app := sein.New()

	app.Post("/verify-age", func(ctx context.Context, req SelfValidatingDTO) (string, error) {
		return "verified", nil
	})

	// 1. Invalid age (15)
	bodyUnderage, err := json.Marshal(SelfValidatingDTO{Age: 15})
	require.NoError(t, err)

	reqUnder := httptest.NewRequest(http.MethodPost, "/verify-age", bytes.NewReader(bodyUnderage))
	recUnder := httptest.NewRecorder()
	app.ServeHTTP(recUnder, reqUnder)

	assert.Equal(t, http.StatusBadRequest, recUnder.Code)

	// 2. Valid age (21)
	bodyAdult, err := json.Marshal(SelfValidatingDTO{Age: 21})
	require.NoError(t, err)

	reqAdult := httptest.NewRequest(http.MethodPost, "/verify-age", bytes.NewReader(bodyAdult))
	recAdult := httptest.NewRecorder()
	app.ServeHTTP(recAdult, reqAdult)

	assert.Equal(t, http.StatusOK, recAdult.Code)
}

type UserSessionData struct {
	AccountID uint64
	Role      string
}

type FullFeaturedDTO struct {
	UserID    uint64           `path:"user_id"`
	TraceID   string           `header:"X-Trace-ID,required"`
	SessionID string           `cookie:"session_id,required"`
	Token     string           `auth:"bearer,required"`
	Session   *UserSessionData `ctx:""`
	Tags      []string         `query:"tag"`
	Limit     int              `query:"limit,default=50"`
	Optional  *string          `query:"opt"`
	Title     string           `json:"title"`
}

func TestUnifiedDTO_FullFeatures(t *testing.T) {
	app := sein.New()

	app.Use(func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			sein.Set(req, &UserSessionData{AccountID: 42, Role: "admin"})
			return next(req)
		}
	})

	app.Post("/users/:user_id/posts", func(ctx context.Context, req FullFeaturedDTO) (map[string]any, error) {
		optVal := ""
		if req.Optional != nil {
			optVal = *req.Optional
		}
		return map[string]any{
			"user_id":    req.UserID,
			"trace_id":   req.TraceID,
			"session_id": req.SessionID,
			"token":      req.Token,
			"account_id": req.Session.AccountID,
			"role":       req.Session.Role,
			"tags":       req.Tags,
			"limit":      req.Limit,
			"optional":   optVal,
			"title":      req.Title,
		}, nil
	})

	bodyJSON, err := json.Marshal(map[string]string{"title": "Zero-Reflection Post"})
	require.NoError(t, err)

	httpReq := httptest.NewRequest(http.MethodPost, "/users/100/posts?tag=go&tag=rust&opt=custom", bytes.NewReader(bodyJSON))
	httpReq.Header.Set("X-Trace-ID", "trace-xyz-777")
	httpReq.Header.Set("Authorization", "Bearer secret-token-abc")
	httpReq.AddCookie(&http.Cookie{Name: "session_id", Value: "sess-cookie-999"})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusOK, rec.Code)

	var res map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))

	assert.Equal(t, float64(100), res["user_id"])
	assert.Equal(t, "trace-xyz-777", res["trace_id"])
	assert.Equal(t, "sess-cookie-999", res["session_id"])
	assert.Equal(t, "secret-token-abc", res["token"])
	assert.Equal(t, float64(42), res["account_id"])
	assert.Equal(t, "admin", res["role"])
	assert.Equal(t, float64(50), res["limit"])
	assert.Equal(t, "custom", res["optional"])
	assert.Equal(t, "Zero-Reflection Post", res["title"])
}

type MismatchedDTO struct {
	WrongParam string `path:"some_other_id"`
}

func TestUnifiedDTO_StartupValidationPanic(t *testing.T) {
	app := sein.New()

	assert.Panics(t, func() {
		app.GetWith("/users/:user_id", func(ctx context.Context, req MismatchedDTO) (string, error) {
			return "ok", nil
		})
	})
}

type AdvancedTransformDTO struct {
	CleanEmail  string        `query:"email,trim,lower,email"`
	CountryCode string        `query:"country,trim,upper,len=2"`
	CleanText   string        `query:"text,single_space"`
	CardNumber  string        `query:"card,digits_only"`
	Status      string        `query:"status,enum=active|pending|archived"`
	CreatedAt   time.Time     `query:"created_at"`
	Timeout     time.Duration `query:"timeout,default=15s"`
	ClientIP    net.IP        `net:"ip"`
	IPAddr      netip.Addr    `net:"ip"`
	Protocol    string        `net:"proto"`
	Host        string        `net:"host"`
}

func TestUnifiedDTO_AdvancedTransformsAndAdapters(t *testing.T) {
	app := sein.New()

	app.GetWith("/analytics", func(ctx context.Context, req AdvancedTransformDTO) (map[string]any, error) {
		return map[string]any{
			"email":      req.CleanEmail,
			"country":    req.CountryCode,
			"text":       req.CleanText,
			"card":       req.CardNumber,
			"status":     req.Status,
			"created_at": req.CreatedAt.Format(time.RFC3339),
			"timeout_ms": req.Timeout.Milliseconds(),
			"client_ip":  req.ClientIP.String(),
			"ip_addr":    req.IPAddr.String(),
			"protocol":   req.Protocol,
			"host":       req.Host,
		}, nil
	})

	url := "/analytics?email=%20%20User.Name@Example.COM%20%20" +
		"&country=us" +
		"&text=Hello%20%20%20World%20%20%20From%20%20Sein" +
		"&card=4111-2222-3333-4444" +
		"&status=active" +
		"&created_at=2026-08-26T12:00:00Z"

	httpReq := httptest.NewRequest(http.MethodGet, url, nil)
	httpReq.Header.Set("X-Real-IP", "203.0.113.195")
	httpReq.Host = "api.sein.dev"

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusOK, rec.Code)

	var res map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))

	assert.Equal(t, "user.name@example.com", res["email"])
	assert.Equal(t, "US", res["country"])
	assert.Equal(t, "Hello World From Sein", res["text"])
	assert.Equal(t, "4111222233334444", res["card"])
	assert.Equal(t, "active", res["status"])
	assert.Equal(t, "2026-08-26T12:00:00Z", res["created_at"])
	assert.Equal(t, float64(15000), res["timeout_ms"])
	assert.Equal(t, "203.0.113.195", res["client_ip"])
	assert.Equal(t, "api.sein.dev", res["host"])
}

type FileUploadDTO struct {
	Category string     `form:"category,required,trim,lower"`
	Avatar   *sein.File `file:"avatar,required"`
}

func TestUnifiedDTO_FileUpload(t *testing.T) {
	app := sein.New()

	app.Post("/upload", func(ctx context.Context, req FileUploadDTO) (map[string]any, error) {
		fileBytes, err := req.Avatar.Bytes()
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"category":  req.Category,
			"filename":  req.Avatar.Filename,
			"size":      req.Avatar.Size,
			"file_data": string(fileBytes),
		}, nil
	})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("category", "  AVATARS  ")

	part, err := writer.CreateFormFile("avatar", "profile.png")
	require.NoError(t, err)
	_, _ = part.Write([]byte("fake-image-bytes-12345"))
	_ = writer.Close()

	httpReq := httptest.NewRequest(http.MethodPost, "/upload", &body)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusOK, rec.Code)

	var res map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))

	assert.Equal(t, "avatars", res["category"])
	assert.Equal(t, "profile.png", res["filename"])
	assert.Equal(t, "fake-image-bytes-12345", res["file_data"])
}

type PydanticFeaturesDTO struct {
	UserID    uuid.UUID           `path:"id,uuid"`
	Username  string              `query:"username,pattern=^[a-z0-9_]{3,16}$"`
	Callback  string              `query:"callback,url"`
	Step      int                 `query:"step,positive,multiple_of=5,le=100"`
	Tags      []string            `query:"tags,sep=|"`
	BinaryHex []byte              `query:"hash,hex"`
	BinaryB64 []byte              `query:"b64,base64"`
	Password  sein.Secret[string] `json:"password"`
}

func TestUnifiedDTO_PydanticGradeFeatures(t *testing.T) {
	app := sein.New()

	app.Post("/users/:id/action", func(ctx context.Context, req PydanticFeaturesDTO) (map[string]any, error) {
		return map[string]any{
			"user_id":     req.UserID.String(),
			"username":    req.Username,
			"callback":    req.Callback,
			"step":        req.Step,
			"tags":        req.Tags,
			"hash_str":    string(req.BinaryHex),
			"b64_str":     string(req.BinaryB64),
			"pass_masked": req.Password.String(),
			"pass_raw":    req.Password.Value(),
		}, nil
	})

	bodyJSON, err := json.Marshal(map[string]string{
		"password": "my-ultra-secret-pass",
	})
	require.NoError(t, err)

	url := "/users/a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11/action" +
		"?username=john_doe_99" +
		"&callback=https://webhook.site/test" +
		"&step=25" +
		"&tags=alpha|beta|gamma" +
		"&hash=68656c6c6f" +
		"&b64=d29ybGQ="

	httpReq := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(bodyJSON))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusOK, rec.Code)

	var res map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))

	assert.Equal(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", res["user_id"])
	assert.Equal(t, "john_doe_99", res["username"])
	assert.Equal(t, "https://webhook.site/test", res["callback"])
	assert.Equal(t, float64(25), res["step"])
	assert.Equal(t, []any{"alpha", "beta", "gamma"}, res["tags"])
	assert.Equal(t, "hello", res["hash_str"])
	assert.Equal(t, "world", res["b64_str"])
	assert.Equal(t, "******", res["pass_masked"])
	assert.Equal(t, "my-ultra-secret-pass", res["pass_raw"])
}

func TestSecretMasking(t *testing.T) {
	s := sein.NewSecret("super-secret-key-123")

	assert.Equal(t, "******", s.String())
	assert.Equal(t, "super-secret-key-123", s.Value())
	assert.Equal(t, "super-secret-key-123", s.Expose())

	// JSON marshaling must be masked
	data, err := json.Marshal(s)
	require.NoError(t, err)
	assert.Equal(t, "\"******\"", string(data))
}

