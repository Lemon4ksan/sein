// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"strings"
	"unicode"
)

// CompileTransforms constructs an array of precompiled transform functions for a field pipeline.
func CompileTransforms(b *FieldBinding) []TransformFunc {
	var transforms []TransformFunc

	if b.Trim {
		transforms = append(transforms, strings.TrimSpace)
	}
	if b.Lower {
		transforms = append(transforms, strings.ToLower)
	}
	if b.Upper {
		transforms = append(transforms, strings.ToUpper)
	}
	if b.SingleSpace {
		transforms = append(transforms, collapseSpaces)
	}
	if b.DigitsOnly {
		transforms = append(transforms, extractDigits)
	}

	return transforms
}

func collapseSpaces(s string) string {
	var buf strings.Builder
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				buf.WriteRune(' ')
				inSpace = true
			}
		} else {
			buf.WriteRune(r)
			inSpace = false
		}
	}
	return strings.TrimSpace(buf.String())
}

func extractDigits(s string) string {
	var buf strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
