// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metadata_test

import (
	"context"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"

	"github.com/lemon4ksan/sein/grpc/metadata"
)

func TestMetadata_Operations(t *testing.T) {
	md := metadata.New(map[string]string{
		"Key1": "val1",
	})
	assert.Equal(t, 1, md.Len())
	assert.Equal(t, []string{"val1"}, md.Get("key1"))

	md.Append("Key1", "val2")
	assert.Equal(t, []string{"val1", "val2"}, md.Get("key1"))

	md.Set("Key2", "val3")
	assert.Equal(t, []string{"val3"}, md.Get("key2"))

	mdCopy := md.Copy()
	assert.Equal(t, 2, mdCopy.Len())

	md.Delete("Key1")
	assert.Nil(t, md.Get("key1"))
	assert.NotNil(t, mdCopy.Get("key1"))
}

func TestMetadata_Context(t *testing.T) {
	ctx := context.Background()

	inMD := metadata.Pairs("auth", "token123")
	ctx = metadata.NewIncomingContext(ctx, inMD)

	gotInMD, ok := metadata.FromIncomingContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, []string{"token123"}, gotInMD.Get("auth"))

	outMD := metadata.Pairs("server-id", "s1")
	ctx = metadata.NewOutgoingContext(ctx, outMD)

	gotOutMD, ok := metadata.FromOutgoingContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, []string{"s1"}, gotOutMD.Get("server-id"))
}
