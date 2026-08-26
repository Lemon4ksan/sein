// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package status_test

import (
	"errors"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"

	"github.com/lemon4ksan/sein/grpc/codes"
	"github.com/lemon4ksan/sein/grpc/status"
)

func TestStatus_Basics(t *testing.T) {
	st := status.New(codes.NotFound, "entity missing")
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Equal(t, "entity missing", st.Message())
	assert.Equal(t, "rpc error: code = NotFound desc = entity missing", st.String())

	err := st.Err()
	assert.Error(t, err)

	fromErr, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.NotFound, fromErr.Code())
	assert.Equal(t, "entity missing", fromErr.Message())

	// Code extractor
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Equal(t, codes.OK, status.Code(nil))
	assert.Equal(t, codes.Unknown, status.Code(errors.New("plain error")))
}

func TestStatus_Is(t *testing.T) {
	err1 := status.Error(codes.AlreadyExists, "duplicate")
	err2 := status.Error(codes.AlreadyExists, "duplicate")
	err3 := status.Error(codes.NotFound, "missing")

	assert.True(t, errors.Is(err1, err2))
	assert.False(t, errors.Is(err1, err3))
}
