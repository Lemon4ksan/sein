// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metadata_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/lemon4ksan/sein/grpc/metadata"
)

func FuzzMetadataCopyToHTTP(f *testing.F) {
	f.Add("x-custom-key", "value1")
	f.Add("authorization", "Bearer secret")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, key, val string) {
		md := metadata.MD{
			key: []string{val},
		}

		ctx := metadata.NewIncomingContext(context.Background(), md)
		if extracted, ok := metadata.FromIncomingContext(ctx); ok {
			hdr := make(http.Header)
			extracted.CopyToHTTP(hdr)
		}
	})
}
