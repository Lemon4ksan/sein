// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package openapi

import (
	"testing"
)

func FuzzOpenAPIPathConversion(f *testing.F) {
	f.Add("/users/:id/posts/:post_id")
	f.Add("/api/v1/*wildcard")
	f.Add("/")
	f.Add("/items/:item_id/details")

	f.Fuzz(func(t *testing.T, path string) {
		_, _ = convertRouteToOpenAPI(path)
	})
}

func FuzzOpenAPITagParsing(f *testing.F) {
	f.Add("field_name,omitempty", "defaultField")
	f.Add("custom_name,required", "name")
	f.Add("-", "secret")
	f.Add("", "fallback")

	f.Fuzz(func(t *testing.T, tag, def string) {
		_ = parseTagName(tag, def)
	})
}
