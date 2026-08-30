// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
)

type ImageDTO struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func TestTypedRedirect_RedirectTo(t *testing.T) {
	app := sein.New()

	app.Get("/image/typed", func(ctx context.Context) (sein.Response[ImageDTO], error) {
		return sein.RedirectTo[ImageDTO]("/storage/image.png", http.StatusTemporaryRedirect), nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/image/typed", nil)
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTemporaryRedirect, rec.Code)
	assert.Equal(t, "/storage/image.png", rec.Header().Get("Location"))
}

func TestTypedRedirect_ErrRedirect(t *testing.T) {
	app := sein.New()

	app.Get("/avatar", func(ctx context.Context) (*ImageDTO, error) {
		// Handler signature returns (*ImageDTO, error) without requiring Response[any]
		return nil, sein.ErrRedirect("https://gravatar.com/avatar/user123", http.StatusSeeOther)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/avatar", nil)
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "https://gravatar.com/avatar/user123", rec.Header().Get("Location"))
	assert.Empty(t, rec.Body.String())
}

func TestTypedRedirect_NativeH1(t *testing.T) {
	app := sein.New()

	app.Get("/go", func(ctx context.Context) (*ImageDTO, error) {
		return nil, sein.ErrRedirect("/dest")
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	go func() { _ = app.Serve(ln) }()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get("http://" + ln.Addr().String() + "/go")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/dest", resp.Header.Get("Location"))
}
