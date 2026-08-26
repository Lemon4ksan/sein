// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prefork_test

import (
	"os"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein/builtin/prefork"
)

func TestIsChild(t *testing.T) {
	_ = os.Unsetenv(prefork.EnvPreforkChild)
	assert.False(t, prefork.IsChild())

	_ = os.Setenv(prefork.EnvPreforkChild, "1")
	defer func() { _ = os.Unsetenv(prefork.EnvPreforkChild) }()
	assert.True(t, prefork.IsChild())
}

func TestListen_BindAndClose(t *testing.T) {
	ln, err := prefork.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NotNil(t, ln)

	addr := ln.Addr().String()
	assert.NotEmpty(t, addr)

	require.NoError(t, ln.Close())
}

func TestOptions(t *testing.T) {
	cfg := prefork.Config{}

	prefork.WithWorkers(8)(&cfg)
	assert.Equal(t, 8, cfg.Workers)

	prefork.WithRestartDelay(100 * time.Millisecond)(&cfg)
	assert.Equal(t, 100*time.Millisecond, cfg.RestartDelay)

	prefork.WithGracefulTimeout(3 * time.Second)(&cfg)
	assert.Equal(t, 3*time.Second, cfg.GracefulTimeout)
}
