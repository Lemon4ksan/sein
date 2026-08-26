package brotli

/* Copyright 2013 Google Inc. All Rights Reserved.

   Distributed under MIT license.
   See file LICENSE for detail or copy at https://opensource.org/licenses/MIT
*/

func brotli_min_double(a, b float64) float64 {
	if a < b {
		return a
	} else {
		return b
	}
}

func brotli_max_double(a, b float64) float64 {
	if a > b {
		return a
	} else {
		return b
	}
}

func brotli_min_float(a, b float32) float32 {
	if a < b {
		return a
	} else {
		return b
	}
}

func brotli_max_float(a, b float32) float32 {
	if a > b {
		return a
	} else {
		return b
	}
}

func brotli_min_int(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func brotli_max_int(a, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}

func brotli_min_size_t(a, b uint) uint {
	if a < b {
		return a
	} else {
		return b
	}
}

func brotli_max_size_t(a, b uint) uint {
	if a > b {
		return a
	} else {
		return b
	}
}

func brotli_min_uint32_t(a, b uint32) uint32 {
	if a < b {
		return a
	} else {
		return b
	}
}

func brotli_max_uint32_t(a, b uint32) uint32 {
	if a > b {
		return a
	} else {
		return b
	}
}

func brotli_min_uint8_t(a, b byte) byte {
	if a < b {
		return a
	} else {
		return b
	}
}

func brotli_max_uint8_t(a, b byte) byte {
	if a > b {
		return a
	} else {
		return b
	}
}

var (
	_ = brotli_min_double
	_ = brotli_max_float
	_ = brotli_min_uint8_t
)
