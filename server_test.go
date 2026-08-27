// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
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
	g.GetWith("/:id", c.getByID)
	g.Post("", c.create)
}

func (c *mockUserController) getByID(ctx context.Context, req struct {
	ID uint64 `path:"id"`
},
) (UserResponse, error) {
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

func TestGroupMapError(t *testing.T) {
	app := sein.New()

	errDatabaseDuplicate := errors.New("db: unique constraint violated")
	errUserNotFound := errors.New("service: user not found")

	api := app.Group("/api/v1")
	users := api.Group("/users")

	// Map errors locally on group
	users.MapError(errDatabaseDuplicate, sein.Conflict("USER_EXISTS", "User already registered")).
		MapError(errUserNotFound, sein.NotFound("USER_NOT_FOUND", "User entity not found"))

	users.Post("/create", func(ctx context.Context, req CreateUserDTO) (UserResponse, error) {
		if req.Email == "duplicate@example.com" {
			return UserResponse{}, errDatabaseDuplicate
		}
		return UserResponse{ID: 1, Name: req.Name}, nil
	})

	type testIDDTO struct {
		ID int `path:"id"`
	}

	users.GetWith("/:id", func(ctx context.Context, req testIDDTO) (UserResponse, error) {
		if req.ID == 999 {
			return UserResponse{}, errUserNotFound
		}
		return UserResponse{ID: uint64(req.ID), Name: "Alice"}, nil
	})

	// Test 1: Conflict from group error mapping
	body, _ := json.Marshal(CreateUserDTO{Name: "Duplicate", Email: "duplicate@example.com"})
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/users/create", bytes.NewReader(body))
	rec1 := httptest.NewRecorder()
	app.ServeHTTP(rec1, req1)

	assert.Equal(t, http.StatusConflict, rec1.Code)
	assert.Contains(t, rec1.Body.String(), "USER_EXISTS")

	// Test 2: NotFound from group error mapping
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/users/999", nil)
	rec2 := httptest.NewRecorder()
	app.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusNotFound, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "USER_NOT_FOUND")
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
	TraceID   string           `               header:"X-Trace-ID,required"`
	SessionID string           `                                            cookie:"session_id,required"`
	Token     string           `                                                                         auth:"bearer,required"`
	Session   *UserSessionData `                                                                                                ctx:""`
	Tags      []string         `                                                                                                       query:"tag"`
	Limit     int              `                                                                                                       query:"limit,default=50"`
	Optional  *string          `                                                                                                       query:"opt"`
	Title     string           `                                                                                                                                json:"title"`
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

	httpReq := httptest.NewRequest(
		http.MethodPost,
		"/users/100/posts?tag=go&tag=rust&opt=custom",
		bytes.NewReader(bodyJSON),
	)
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
	ClientIP    net.IP        `                                            net:"ip"`
	IPAddr      netip.Addr    `                                            net:"ip"`
	Protocol    string        `                                            net:"proto"`
	Host        string        `                                            net:"host"`
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
	Avatar   *sein.File `                                    file:"avatar,required"`
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
	Username  string              `               query:"username,pattern=^[a-z0-9_]{3,16}$"`
	Callback  string              `               query:"callback,url"`
	Step      int                 `               query:"step,positive,multiple_of=5,le=100"`
	Tags      []string            `               query:"tags,sep=|"`
	BinaryHex []byte              `               query:"hash,hex"`
	BinaryB64 []byte              `               query:"b64,base64"`
	Password  sein.Secret[string] `                                                          json:"password"`
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

func TestNativeH1Server_SocketAndKeepAlive(t *testing.T) {
	app := sein.New()

	type CreateUserReq struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	type UserResp struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	app.Post("/users", func(ctx context.Context, req CreateUserReq) (UserResp, error) {
		return UserResp{
			ID:    42,
			Name:  req.Name,
			Email: req.Email,
		}, nil
	})

	type GetUserDTO struct {
		ID int `path:"id"`
	}

	app.GetWith("/users/:id", func(ctx context.Context, req GetUserDTO) (UserResp, error) {
		if req.ID == 42 {
			return UserResp{ID: 42, Name: "Bob", Email: "bob@example.com"}, nil
		}

		return UserResp{}, sein.NotFound("USER_NOT_FOUND", "User not found")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()

	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. POST /users
	postBody, _ := json.Marshal(CreateUserReq{Name: "Bob", Email: "bob@example.com"})
	resp, err := client.Post("http://"+addr+"/users", "application/json", bytes.NewReader(postBody))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var created UserResp
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	_ = resp.Body.Close()

	assert.Equal(t, 42, created.ID)
	assert.Equal(t, "Bob", created.Name)

	// 2. GET /users/42 (Keep-Alive reused connection)
	resp2, err := client.Get("http://" + addr + "/users/42")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	var fetched UserResp
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&fetched))
	_ = resp2.Body.Close()

	assert.Equal(t, 42, fetched.ID)

	// 3. GET /users/999 (404 Domain Error)
	resp3, err := client.Get("http://" + addr + "/users/999")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp3.StatusCode)
	_ = resp3.Body.Close()

	// 4. Graceful Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, app.Shutdown(ctx))
}

func TestNativeH1Server_StreamWriterAndSSE(t *testing.T) {
	app := sein.New()

	app.Get("/stream", func(ctx context.Context) (sein.StreamWriterResponse, error) {
		return sein.StreamWriter(func(w io.Writer) error {
			_, _ = w.Write([]byte("chunk-alpha-"))
			_, _ = w.Write([]byte("chunk-beta"))
			return nil
		}), nil
	})

	app.Get("/sse", func(ctx context.Context) (sein.SSEResponse, error) {
		return sein.SSE(func(s *sein.SSESender) error {
			_ = s.SendEvent("update", "state_ready")
			_ = s.SendJSON("metric", map[string]int{"cpu": 45})
			return nil
		}), nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()

	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Test /stream (Chunked output)
	respStream, err := client.Get("http://" + addr + "/stream")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respStream.StatusCode)
	streamBody, err := io.ReadAll(respStream.Body)
	require.NoError(t, err)

	_ = respStream.Body.Close()

	assert.Equal(t, "chunk-alpha-chunk-beta", string(streamBody))

	// 2. Test /sse (Event stream)
	respSSE, err := client.Get("http://" + addr + "/sse")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respSSE.StatusCode)
	assert.Equal(t, "text/event-stream", respSSE.Header.Get("Content-Type"))
	sseBody, err := io.ReadAll(respSSE.Body)
	require.NoError(t, err)

	_ = respSSE.Body.Close()

	assert.Contains(t, string(sseBody), "event: update\ndata: state_ready\n\n")
	assert.Contains(t, string(sseBody), "event: metric\ndata: {\"cpu\":45}\n\n")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, app.Shutdown(ctx))
}

func TestNativeH1Server_MultipartFileUpload(t *testing.T) {
	app := sein.New()

	type UploadDTO struct {
		Title string     `form:"title"`
		Doc   *sein.File `form:"doc"`
	}

	app.PostWith("/upload", func(ctx context.Context, req UploadDTO) (map[string]any, error) {
		docBytes, err := req.Doc.Bytes()
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"title":    req.Title,
			"filename": req.Doc.Filename,
			"size":     req.Doc.Size,
			"content":  string(docBytes),
		}, nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()

	go func() {
		_ = app.Serve(ln)
	}()

	// Build multipart request body
	var b bytes.Buffer

	w := multipart.NewWriter(&b)
	_ = w.WriteField("title", "Quarterly Report")
	part, _ := w.CreateFormFile("doc", "report.txt")
	_, _ = part.Write([]byte("Confidential corporate data 2026"))
	_ = w.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post("http://"+addr+"/upload", w.FormDataContentType(), &b)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	_ = resp.Body.Close()

	assert.Equal(t, "Quarterly Report", result["title"])
	assert.Equal(t, "report.txt", result["filename"])
	assert.Equal(t, float64(len("Confidential corporate data 2026")), result["size"])
	assert.Equal(t, "Confidential corporate data 2026", result["content"])

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, app.Shutdown(ctx))
}

func TestNativeH1Server_ConditionalETagAnd304(t *testing.T) {
	app := sein.New()

	const currentETag = "\"v1.0.0-hash\""

	app.GetReq("/config", func(req *sein.Request) (sein.Response[any], error) {
		if req.IfNoneMatch(currentETag) {
			return sein.NotModified().WithETag(currentETag), nil
		}

		return sein.OK[any](map[string]string{"theme": "dark"}).WithETag(currentETag), nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()

	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Initial request without ETag (gets 200 OK and ETag header)
	resp1, err := client.Get("http://" + addr + "/config")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp1.StatusCode)
	assert.Equal(t, currentETag, resp1.Header.Get("ETag"))
	_ = resp1.Body.Close()

	// 2. Subsequent request WITH If-None-Match matching currentETag (gets 304 Not Modified)
	req2, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/config", nil)
	req2.Header.Set("If-None-Match", currentETag)
	resp2, err := client.Do(req2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotModified, resp2.StatusCode)
	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	assert.Empty(t, body2)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, app.Shutdown(ctx))
}

func TestSkipUnmatchedRoutes(t *testing.T) {
	var (
		mwExecutedCount atomic.Int64
	)

	trackingMW := func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			mwExecutedCount.Add(1)
			return next(req)
		}
	}

	app := sein.New(sein.WithSkipUnmatchedRoutes(true))
	app.Use(trackingMW)

	app.Get("/active", func(ctx context.Context) (string, error) {
		return "OK", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Matched route -> middleware executed
	resp1, err := client.Get("http://" + addr + "/active")
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)
	assert.Equal(t, int64(1), mwExecutedCount.Load())

	// 2. Unmatched route (404) -> middleware SKIPPED
	resp2, err := client.Get("http://" + addr + "/non-existent-endpoint")
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp2.StatusCode)
	// mwExecutedCount must remain 1
	assert.Equal(t, int64(1), mwExecutedCount.Load())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}

type mockUserModule struct{}

func (m mockUserModule) Mount(g *sein.Group) {
	g.Get("/profile", func(ctx context.Context) (string, error) {
		return "user-profile", nil
	})
	g.Post("/settings", func(ctx context.Context, req map[string]string) (string, error) {
		return "settings-saved", nil
	})
}

func TestModularMounting(t *testing.T) {
	app := sein.New()

	// 1. Mount struct module
	app.Mount("/users", mockUserModule{})

	// 2. Mount functional module
	app.Mount("/bots", sein.ModuleFunc(func(g *sein.Group) {
		g.Get("/status", func(ctx context.Context) (string, error) {
			return "bots-running", nil
		})
	}))

	// Test /users/profile
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/users/profile", nil)
	app.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, "user-profile", rec1.Body.String())

	// Test /bots/status
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/bots/status", nil)
	app.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Equal(t, "bots-running", rec2.Body.String())
}

func TestRouteIntrospection(t *testing.T) {
	app := sein.New()

	app.Get("/health", func(ctx context.Context) (string, error) {
		return "ok", nil
	})
	app.Post("/auth/login", func(ctx context.Context, req map[string]string) (string, error) {
		return "token", nil
	})
	app.Mount("/users", mockUserModule{})

	routes := app.Routes()
	require.Len(t, routes, 4)

	assert.Equal(t, "GET", routes[0].Method)
	assert.Equal(t, "/health", routes[0].Path)

	assert.Equal(t, "POST", routes[1].Method)
	assert.Equal(t, "/auth/login", routes[1].Path)

	assert.Equal(t, "GET", routes[2].Method)
	assert.Equal(t, "/users/profile", routes[2].Path)

	assert.Equal(t, "POST", routes[3].Method)
	assert.Equal(t, "/users/settings", routes[3].Path)

	table := app.PrintRoutes()
	assert.Contains(t, table, "GET")
	assert.Contains(t, table, "/users/profile")
	assert.Contains(t, table, "/auth/login")
}

func TestGuardScope(t *testing.T) {
	app := sein.New()

	authMiddleware := func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			if req.Header("Authorization") != "Bearer secret" {
				return nil, sein.Unauthorized("UNAUTHORIZED", "Missing or invalid token")
			}
			return next(req)
		}
	}

	// 1. Guard with Mount
	app.Guard(authMiddleware).Mount("/users", mockUserModule{})

	// 2. Guard with Do block
	app.Guard(authMiddleware).Do(func(g *sein.Group) {
		g.Get("/secret-data", func(ctx context.Context) (string, error) {
			return "top-secret", nil
		})
	})

	// Test unauthenticated access to /users/profile -> 401
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/users/profile", nil)
	app.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusUnauthorized, rec1.Code)

	// Test authenticated access to /users/profile -> 200
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/users/profile", nil)
	req2.Header.Set("Authorization", "Bearer secret")
	app.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Equal(t, "user-profile", rec2.Body.String())

	// Test authenticated access to /secret-data -> 200
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/secret-data", nil)
	req3.Header.Set("Authorization", "Bearer secret")
	app.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusOK, rec3.Code)
	assert.Equal(t, "top-secret", rec3.Body.String())
}

func TestAfterResponseHook(t *testing.T) {
	app := sein.New()

	var recordedPath string
	var recordedStatus int
	var recordedDuration time.Duration

	app.AfterResponse(func(req *sein.Request, statusCode int, duration time.Duration) {
		recordedPath = req.Path()
		recordedStatus = statusCode
		recordedDuration = duration
	})

	app.Get("/ping", func(ctx context.Context) (string, error) {
		return "pong", nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "pong", rec.Body.String())
	assert.Equal(t, "/ping", recordedPath)
	assert.Equal(t, http.StatusOK, recordedStatus)
	assert.True(t, recordedDuration >= 0)
}

type TestUserSession struct {
	UserID int
	Name   string
}

func TestDerive(t *testing.T) {
	app := sein.New()

	sein.Derive(app, func(req *sein.Request) (*TestUserSession, error) {
		token := req.Header("X-Token")
		if token != "valid-token" {
			return nil, sein.Unauthorized("INVALID_TOKEN", "Token is invalid")
		}
		return &TestUserSession{UserID: 101, Name: "Alice"}, nil
	})

	app.GetAuth("/me", func(ctx context.Context, session *TestUserSession) (string, error) {
		return "Hello, " + session.Name, nil
	})

	// 1. Unauthorized request
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/me", nil)
	app.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusUnauthorized, rec1.Code)

	// 2. Authorized request
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/me", nil)
	req2.Header.Set("X-Token", "valid-token")
	app.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Equal(t, "Hello, Alice", rec2.Body.String())
}

func TestMicroTrace(t *testing.T) {
	app := sein.New()

	var traceRecorded *sein.TraceInfo
	app.Trace(func(tr *sein.TraceInfo) {
		traceRecorded = tr
	})

	app.Get("/hello", func(ctx context.Context) (string, error) {
		return "world", nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, traceRecorded)
	assert.Equal(t, "GET", traceRecorded.Method)
	assert.Equal(t, "/hello", traceRecorded.Path)
	assert.Equal(t, http.StatusOK, traceRecorded.StatusCode)
	assert.True(t, traceRecorded.TotalDuration >= 0)
}
