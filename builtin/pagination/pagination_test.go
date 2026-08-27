// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pagination_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/builtin/pagination"
)

func TestPagination_ExtractionAndMetadata(t *testing.T) {
	app := sein.New()
	app.Use(pagination.New(
		pagination.WithDefaultLimit(10),
		pagination.WithMaxLimit(50),
	))

	app.Get("/products", func(req *sein.Request) (any, error) {
		p := pagination.From(req)
		items := []string{"item1", "item2"}
		return pagination.NewPaginated(items, 45, p), nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	go func() {
		_ = app.Serve(ln)
	}()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. GET /products?page=2&limit=10&sort=created_at&order=desc
	resp, err := client.Get("http://" + addr + "/products?page=2&limit=10&sort=created_at&order=desc")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)

	var res pagination.Paginated[string]
	require.NoError(t, json.Unmarshal(body, &res))

	assert.Equal(t, 2, len(res.Items))
	assert.Equal(t, 2, res.Meta.Page)
	assert.Equal(t, 10, res.Meta.Limit)
	assert.Equal(t, int64(45), res.Meta.TotalItems)
	assert.Equal(t, 5, res.Meta.TotalPages)
	assert.True(t, res.Meta.HasNext)
	assert.True(t, res.Meta.HasPrev)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, app.Shutdown(ctx))
}
