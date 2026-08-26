// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package prefork

import (
	"context"
	"net"
	"syscall"
)

// Listen creates a TCP listener on Windows with socket reuse options.
func Listen(network, address string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error

			err := c.Control(func(fd uintptr) {
				opErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
			if err != nil {
				return err
			}

			return opErr
		},
	}

	return lc.Listen(context.Background(), network, address)
}
