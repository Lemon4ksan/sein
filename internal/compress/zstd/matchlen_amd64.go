//go:build amd64 && !appengine && !noasm && gc

// Copyright (c) 2019-2023 Klaus Post. All rights reserved.
// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zstd

// matchLen returns how many bytes match in a and b
//
// It assumes that:
//
//	len(a) <= len(b) and len(a) > 0
//
//go:noescape
func matchLen(a, b []byte) int
