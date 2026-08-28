// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasip1 || wasip2 || js || wasm || plan9 || solaris || illumos || aix || (!windows && !linux && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd)

package prefork

import "net"

// Listen creates a standard TCP listener on platforms without SO_REUSEPORT support.
func Listen(network, address string) (net.Listener, error) {
	return net.Listen(network, address)
}
