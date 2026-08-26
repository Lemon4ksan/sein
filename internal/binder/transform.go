// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"strings"
	"unicode"
)

// ApplyTransforms executes string transformation pipelines (trim, lower, upper, singleSpace, digitsOnly).
func ApplyTransforms(s string, b *FieldBinding) string {
	if b.Trim {
		s = strings.TrimSpace(s)
	}
	if b.Lower {
		s = strings.ToLower(s)
	}
	if b.Upper {
		s = strings.ToUpper(s)
	}
	if b.SingleSpace {
		s = collapseSpaces(s)
	}
	if b.DigitsOnly {
		s = extractDigits(s)
	}
	return s
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
