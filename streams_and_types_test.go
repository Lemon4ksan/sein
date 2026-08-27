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
