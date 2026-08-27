// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/internal/fast/h1engine"
)

func TestSecret_MaskingAndExposure(t *testing.T) {
	sec := sein.NewSecret("super-secret-token-12345")

	// 1. Expose / Value
	assert.Equal(t, "super-secret-token-12345", sec.Value())
	assert.Equal(t, "super-secret-token-12345", sec.Expose())

	// 2. String & GoString formatting
	assert.Equal(t, "******", sec.String())
	assert.Equal(t, "sein.Secret(******)", sec.GoString())
	assert.Equal(t, "Secret: ******", fmt.Sprintf("Secret: %s", sec))
	assert.Equal(t, "Secret: ******", fmt.Sprintf("Secret: %v", sec))

	// 3. JSON serialization masks the output
	data, err := json.Marshal(map[string]any{
		"token": sec,
	})
	require.NoError(t, err)
	assert.Contains(t, string(data), `"token":"******"`)
	assert.NotContains(t, string(data), "super-secret-token-12345")

	// 4. JSON deserialization loads into Secret container
	var decoded struct {
		Token sein.Secret[string] `json:"token"`
	}
	err = json.Unmarshal([]byte(`{"token":"deserialized-secret"}`), &decoded)
	require.NoError(t, err)
	assert.Equal(t, "deserialized-secret", decoded.Token.Value())
}

func TestFile_Operations(t *testing.T) {
	// Create a multipart form file in memory
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("upload", "test_file.txt")
	require.NoError(t, err)
	_, _ = fw.Write([]byte("content of test uploaded file"))
	_ = mw.Close()

	httpReq, err := http.NewRequest(http.MethodPost, "/upload", &body)
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())

	err = httpReq.ParseMultipartForm(1024 * 1024)
	require.NoError(t, err)

	fh := httpReq.MultipartForm.File["upload"][0]
	file := sein.NewFile(fh)

	assert.Equal(t, "test_file.txt", file.Filename)
	assert.Greater(t, file.Size, int64(0))

	// Read bytes
	b, err := file.Bytes()
	require.NoError(t, err)
	assert.Equal(t, "content of test uploaded file", string(b))

	// Cached bytes return identical slice
	bCached, err := file.Bytes()
	require.NoError(t, err)
	assert.Equal(t, string(b), string(bCached))

	// Save to disk via SaveTo
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "saved.txt")
	err = file.SaveTo(targetPath)
	require.NoError(t, err)

	savedData, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Equal(t, "content of test uploaded file", string(savedData))
}

func TestStreamWriterResponse(t *testing.T) {
	sw := sein.StreamWriter(func(w io.Writer) error {
		_, err := w.Write([]byte("chunk-1\nchunk-2\n"))
		return err
	}).WithHeader("X-Stream-ID", "12345").WithContentType("text/plain")

	// 1. WriteToH1
	h1Res := &h1engine.Response{
		Headers: h1engine.NewHeadersWithCapacity(4),
	}
	err := sw.WriteToH1(h1Res)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, h1Res.StatusCode)
	assert.NotNil(t, h1Res.StreamWriter)

	// 2. WriteResponse (net/http fallback)
	rec := httptest.NewRecorder()
	err = sw.WriteResponse(rec)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "chunk-1\nchunk-2\n", rec.Body.String())
	assert.Equal(t, "12345", rec.Header().Get("X-Stream-ID"))
}

func TestStatusCodesAndErrors(t *testing.T) {
	// Error constructors
	err400 := sein.ErrBadRequest("invalid field input")
	assert.Equal(t, http.StatusBadRequest, err400.Status)
	assert.Equal(t, "invalid field input", err400.Message)
	assert.Contains(t, err400.Error(), "invalid field input")

	err401 := sein.ErrUnauthorized("missing token")
	assert.Equal(t, http.StatusUnauthorized, err401.Status)

	err403 := sein.ErrForbidden("access denied")
	assert.Equal(t, http.StatusForbidden, err403.Status)

	err404 := sein.ErrNotFound("user not found")
	assert.Equal(t, http.StatusNotFound, err404.Status)

	err409 := sein.ErrConflict("resource already exists")
	assert.Equal(t, http.StatusConflict, err409.Status)

	err500 := sein.ErrInternal("database explosion")
	assert.Equal(t, http.StatusInternalServerError, err500.Status)

	err504 := sein.ErrGatewayTimeout("upstream timeout")
	assert.Equal(t, http.StatusGatewayTimeout, err504.Status)

	err422 := sein.ErrUnprocessable("unprocessable entity")
	assert.Equal(t, http.StatusUnprocessableEntity, err422.Status)

	err425 := sein.ErrTooEarly("too early")
	assert.Equal(t, http.StatusTooEarly, err425.Status)

	err429 := sein.ErrTooManyRequests("too many requests")
	assert.Equal(t, http.StatusTooManyRequests, err429.Status)

	err413 := sein.ErrRequestEntityTooLarge("payload too large")
	assert.Equal(t, http.StatusRequestEntityTooLarge, err413.Status)

	// NewHTTPError and AsHTTPError
	httpErr := sein.NewHTTPError(http.StatusTeapot, "TEAPOT", "I'm a teapot")
	assert.Equal(t, http.StatusTeapot, httpErr.HTTPStatus())
	assert.Equal(t, "TEAPOT", httpErr.ErrorCode())

	unwrapped, ok := sein.AsHTTPError(httpErr)
	assert.True(t, ok)
	assert.Equal(t, "TEAPOT", unwrapped.Code)
}

func TestVersionMatrix_ErrorMappers_And_Prefix(t *testing.T) {
	app := sein.New()

	targetErr := errors.New("custom-domain-err")
	domainErr := sein.DefineError(http.StatusPaymentRequired, "PAYMENT_REQUIRED", "Please top up balance")

	vm := app.Versioned("1", "2")
	vm.MapError(targetErr, domainErr)

	vm.Get("/test-map", func(ctx context.Context) (string, error) {
		return "", targetErr
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/test-map", nil)
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusPaymentRequired, rec.Code)
	assert.Contains(t, rec.Body.String(), "PAYMENT_REQUIRED")
}

func TestResponse_Constructors_And_SignedCookies(t *testing.T) {
	// 1. Response constructors
	resp201 := sein.Created(map[string]string{"id": "new"})
	assert.Equal(t, http.StatusCreated, resp201.StatusCode())

	resp202 := sein.Accepted("processing")
	assert.Equal(t, http.StatusAccepted, resp202.StatusCode())

	resp204 := sein.NoContent()
	assert.Equal(t, http.StatusNoContent, resp204.StatusCode())

	resp304 := sein.NotModified()
	assert.Equal(t, http.StatusNotModified, resp304.StatusCode())

	resp302 := sein.Redirect("/dashboard")
	assert.Equal(t, http.StatusFound, resp302.StatusCode())
	assert.Equal(t, "/dashboard", resp302.ResponseHeaders().Get("Location"))

	respHTML := sein.HTML("<h1>Hello</h1>")
	assert.Equal(t, "text/html; charset=utf-8", respHTML.ResponseHeaders().Get("Content-Type"))

	// 2. ETag & LastModified
	now := time.Now()
	respWithHeaders := sein.OK("data").
		WithETag("12345").
		WithLastModified(now)
	assert.Equal(t, "\"12345\"", respWithHeaders.ResponseHeaders().Get("ETag"))
	assert.NotEmpty(t, respWithHeaders.ResponseHeaders().Get("Last-Modified"))

	// 3. Cookie signing and verification
	secret := "test-cookie-signing-secret"
	signed := sein.SignCookieValue("session-user-99", secret)
	assert.Contains(t, signed, "session-user-99.")

	verified, ok := sein.VerifyCookieValue(signed, secret)
	assert.True(t, ok)
	assert.Equal(t, "session-user-99", verified)

	_, badSig := sein.VerifyCookieValue(signed+".tampered", secret)
	assert.False(t, badSig)

	_, badFmt := sein.VerifyCookieValue("no-dot-here", secret)
	assert.False(t, badFmt)
}

func TestGroup_Mount_And_Guard(t *testing.T) {
	app := sein.New()

	api := app.Group("/api")

	// Module mounting
	userModule := sein.ModuleFunc(func(g *sein.Group) {
		g.Get("/users", func(ctx context.Context) (string, error) {
			return "users-list", nil
		})
	})
	api.Mount("/v1", userModule)

	// Guard scope with middleware
	var guardHit bool
	guardMW := func(next sein.RawHandler) sein.RawHandler {
		return func(req *sein.Request) (any, error) {
			guardHit = true
			return next(req)
		}
	}

	adminGuard := api.Guard(guardMW)
	adminGuard.Get("/admin", func(ctx context.Context) (string, error) {
		return "admin-secret", nil
	})

	// 1. Mount test
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	app.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Contains(t, rec1.Body.String(), "users-list")

	// 2. Guard test
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	app.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.True(t, guardHit)
}

func TestParams_Overflow_And_GenericGetters(t *testing.T) {
	var p sein.Params

	// Fill up 8 inline slots
	for i := 1; i <= 8; i++ {
		p.Set(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
	}
	// Add 9th and 10th into extra heap overflow
	p.Set("k9", "v9")
	p.Set("k10", "12345")

	assert.Equal(t, "v1", p.Get("k1"))
	assert.Equal(t, "v8", p.Get("k8"))
	assert.Equal(t, "v9", p.Get("k9"))
	assert.Equal(t, "12345", p.Get("k10"))

	val, found := p.Find("k9")
	assert.True(t, found)
	assert.Equal(t, "v9", val)

	_, notFound := p.Find("nonexistent")
	assert.False(t, notFound)
}
