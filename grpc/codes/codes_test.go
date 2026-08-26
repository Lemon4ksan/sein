// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codes_test

import (
	"testing"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein/grpc/codes"
)

func TestCodes_String(t *testing.T) {
	assert.Equal(t, "OK", codes.OK.String())
	assert.Equal(t, "NotFound", codes.NotFound.String())
	assert.Equal(t, "InvalidArgument", codes.InvalidArgument.String())
	assert.Equal(t, "Code(99)", codes.Code(99).String())
}

func TestCodes_JSON(t *testing.T) {
	var c codes.Code

	err := json.Unmarshal([]byte(`"NOT_FOUND"`), &c)
	require.NoError(t, err)
	assert.Equal(t, codes.NotFound, c)

	err = json.Unmarshal([]byte(`5`), &c)
	require.NoError(t, err)
	assert.Equal(t, codes.NotFound, c)
}
