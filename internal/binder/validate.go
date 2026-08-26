// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ErrValidationFailed is returned when declarative constraints fail.
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

// ValidateConstraints validates string length, email syntax, enums, and min/max.
func ValidateConstraints(s string, b *FieldBinding) error {
	if b.HasLen && len(s) != b.LenVal {
		return ValidationError{Message: fmt.Sprintf("%s length must be exactly %d", b.Key, b.LenVal)}
	}

	if b.IsEmail && s != "" {
		if !strings.Contains(s, "@") || !strings.Contains(s, ".") {
			return ValidationError{Message: fmt.Sprintf("%s must be a valid email address", b.Key)}
		}
	}

	if len(b.EnumVals) > 0 && s != "" {
		found := false
		for _, ev := range b.EnumVals {
			if strings.EqualFold(s, ev) {
				found = true
				break
			}
		}
		if !found {
			return ValidationError{Message: fmt.Sprintf("%s must be one of [%s], got %q", b.Key, strings.Join(b.EnumVals, ", "), s)}
		}
	}

	if b.HasMin || b.HasMax {
		targetKind := b.Kind
		if b.IsPtr {
			targetKind = b.ElemKind
		}

		if targetKind == reflect.String {
			strLen := float64(len(s))
			if b.HasMin && strLen < b.MinVal {
				return ValidationError{Message: fmt.Sprintf("%s length must be at least %v", b.Key, b.MinVal)}
			}
			if b.HasMax && strLen > b.MaxVal {
				return ValidationError{Message: fmt.Sprintf("%s length must be at most %v", b.Key, b.MaxVal)}
			}
		} else if isNumericKind(targetKind) && s != "" {
			if num, err := strconv.ParseFloat(s, 64); err == nil {
				if b.HasMin && num < b.MinVal {
					return ValidationError{Message: fmt.Sprintf("%s value must be at least %v", b.Key, b.MinVal)}
				}
				if b.HasMax && num > b.MaxVal {
					return ValidationError{Message: fmt.Sprintf("%s value must be at most %v", b.Key, b.MaxVal)}
				}
			}
		}
	}

	return nil
}

func isNumericKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// RunValidation triggers the Validatable interface on dest if implemented.
func RunValidation(dest any) error {
	if v, ok := dest.(Validatable); ok {
		if err := v.Validate(); err != nil {
			return err
		}
	}
	return nil
}
