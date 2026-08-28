// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (linux || darwin || dragonfly || freebsd || netbsd || openbsd) && !wasip1 && !wasip2 && !js && !wasm

package prefork

import (
	"context"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// Listen creates a high-performance TCP listener configured with SO_REUSEPORT,
// enabling multiple isolated worker processes to bind to the same listening port concurrently.
func Listen(network, address string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error

			err := c.Control(func(fd uintptr) {
				opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})
			if err != nil {
				return err
			}

			return opErr
		},
	}

	return lc.Listen(context.Background(), network, address)
}
