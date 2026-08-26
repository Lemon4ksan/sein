// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h2engine

//go:generate c2plan9 -c ../../../csrc/h2_frame.c -o frame_amd64.s -stub frame_amd64.go -pkg h2engine
//go:generate c2plan9 -c ../../../csrc/hpack_huffman.c -o huffman_amd64.s -stub huffman_amd64.go -pkg h2engine
//go:generate c2plan9 -c ../../../csrc/hpack_qpack_int.c -o varint_amd64.s -stub varint_amd64.go -pkg h2engine
