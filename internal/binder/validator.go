// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/refkit"
)

// ValidationError indicates a failed declarative validation rule.
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

// CompileValidators builds an array of precompiled validator functions for a field pipeline.
func CompileValidators(b *FieldBinding) []ValidatorFunc {
	var validators []ValidatorFunc

	if b.HasLen {
		lenVal := b.LenVal
		key := b.Key
		validators = append(validators, func(s string) error {
			if len(s) != lenVal {
				return ValidationError{Message: fmt.Sprintf("%s length must be exactly %d", key, lenVal)}
			}
			return nil
		})
	}

	if b.IsEmail {
		key := b.Key
		validators = append(validators, func(s string) error {
			if s != "" && (!strings.Contains(s, "@") || !strings.Contains(s, ".")) {
				return ValidationError{Message: fmt.Sprintf("%s must be a valid email address", key)}
			}
			return nil
		})
	}

	if len(b.EnumVals) > 0 {
		lowerEnums := generic.Map(b.EnumVals, strings.ToLower)
		enumSet := generic.NewSet(lowerEnums...)
		key := b.Key
		enumListStr := strings.Join(b.EnumVals, ", ")

		validators = append(validators, func(s string) error {
			if s == "" {
				return nil
			}
			if !enumSet.Has(strings.ToLower(s)) {
				return ValidationError{Message: fmt.Sprintf("%s must be one of [%s], got %q", key, enumListStr, s)}
			}
			return nil
		})
	}

	targetKind := b.Kind
	if b.IsPtr {
		targetKind = b.ElemKind
	}

	if b.HasMin {
		minVal := b.MinVal
		key := b.Key
		if targetKind == reflect.String {
			validators = append(validators, func(s string) error {
				if float64(len(s)) < minVal {
					return ValidationError{Message: fmt.Sprintf("%s length must be at least %v", key, minVal)}
				}
				return nil
			})
		} else if refkit.IsNumeric(targetKind) {
			validators = append(validators, func(s string) error {
				if s == "" {
					return nil
				}
				if num, err := strconv.ParseFloat(s, 64); err == nil {
					if num < minVal {
						return ValidationError{Message: fmt.Sprintf("%s value must be at least %v", key, minVal)}
					}
				}
				return nil
			})
		}
	}

	if b.HasMax {
		maxVal := b.MaxVal
		key := b.Key
		if targetKind == reflect.String {
			validators = append(validators, func(s string) error {
				if float64(len(s)) > maxVal {
					return ValidationError{Message: fmt.Sprintf("%s length must be at most %v", key, maxVal)}
				}
				return nil
			})
		} else if refkit.IsNumeric(targetKind) {
			validators = append(validators, func(s string) error {
				if s == "" {
					return nil
				}
				if num, err := strconv.ParseFloat(s, 64); err == nil {
					if num > maxVal {
						return ValidationError{Message: fmt.Sprintf("%s value must be at most %v", key, maxVal)}
					}
				}
				return nil
			})
		}
	}

	return validators
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
