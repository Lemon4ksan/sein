// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !windows

package prefork

import (
	"context"
	"net"
	"syscall"
)

const soReusePort = 0x0F // unix.SO_REUSEPORT on Linux/Darwin

// Listen creates a high-performance TCP listener configured with SO_REUSEPORT,
// enabling multiple isolated worker processes to bind to the same listening port concurrently.
func Listen(network, address string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error

			err := c.Control(func(fd uintptr) {
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, soReusePort, 1)
			})
			if err != nil {
				return err
			}

			return opErr
		},
	}

	return lc.Listen(context.Background(), network, address)
}
