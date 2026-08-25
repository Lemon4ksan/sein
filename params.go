// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"strconv"
)

// ParamValue represents a raw path parameter or query string value.
type ParamValue string

// String returns the raw string value.
func (p ParamValue) String() string {
	return string(p)
}

// IsEmpty reports whether the parameter is empty.
func (p ParamValue) IsEmpty() bool {
	return len(p) == 0
}

// Int parses the parameter into an integer.
func (p ParamValue) Int() (int, error) {
	return strconv.Atoi(string(p))
}

// AsInt returns the parsed integer or the fallback default if parsing fails.
func (p ParamValue) AsInt(fallback ...int) int {
	v, err := p.Int()
	if err != nil {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return 0
	}
	return v
}

// Uint64 parses the parameter into a uint64.
func (p ParamValue) Uint64() (uint64, error) {
	return strconv.ParseUint(string(p), 10, 64)
}

// AsUint64 returns the parsed uint64 or the fallback default if parsing fails.
func (p ParamValue) AsUint64(fallback ...uint64) uint64 {
	v, err := p.Uint64()
	if err != nil {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return 0
	}
	return v
}

// Int64 parses the parameter into an int64.
func (p ParamValue) Int64() (int64, error) {
	return strconv.ParseInt(string(p), 10, 64)
}

// AsInt64 returns the parsed int64 or the fallback default if parsing fails.
func (p ParamValue) AsInt64(fallback ...int64) int64 {
	v, err := p.Int64()
	if err != nil {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return 0
	}
	return v
}

// Bool parses the parameter into a boolean.
func (p ParamValue) Bool() (bool, error) {
	return strconv.ParseBool(string(p))
}

// AsBool returns the parsed bool or the fallback default if parsing fails.
func (p ParamValue) AsBool(fallback ...bool) bool {
	v, err := p.Bool()
	if err != nil {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return false
	}
	return v
}
