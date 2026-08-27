// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rpc

import (
	"testing"
)

type FuzzPayloadDTO struct {
	ID   int    `path:"id"`
	Slug string `path:"slug"`
}

func FuzzResolvePathParams(f *testing.F) {
	f.Add("/users/:id/posts/:slug", 42, "hello-world")
	f.Add("/api/:unknown", 100, "test")
	f.Add("/", 0, "")

	f.Fuzz(func(t *testing.T, pattern string, id int, slug string) {
		dto := FuzzPayloadDTO{ID: id, Slug: slug}
		_ = resolvePathParams(pattern, dto)
	})
}
