// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"fmt"
	"math"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/refkit"
	"github.com/lemon4ksan/foundation/types/uuid"
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

	key := b.Key

	// 1. Length constraint
	if b.HasLen {
		lenVal := b.LenVal

		validators = append(validators, func(s string) error {
			if len(s) != lenVal {
				return ValidationError{Message: fmt.Sprintf("%s length must be exactly %d", key, lenVal)}
			}

			return nil
		})
	}

	// 2. Precompiled Regex Pattern Matching
	if b.Pattern != "" {
		pat := regexp.MustCompile(b.Pattern)
		patternStr := b.Pattern

		validators = append(validators, func(s string) error {
			if s != "" && !pat.MatchString(s) {
				return ValidationError{Message: fmt.Sprintf("%s must match pattern %q", key, patternStr)}
			}

			return nil
		})
	}

	// 3. Email Validation
	if b.IsEmail {
		validators = append(validators, func(s string) error {
			if s != "" && (!strings.Contains(s, "@") || !strings.Contains(s, ".")) {
				return ValidationError{Message: key + " must be a valid email address"}
			}

			return nil
		})
	}

	// 4. UUID Validation (using SIMD/LLVM foundation/types/uuid)
	if b.IsUUID {
		validators = append(validators, func(s string) error {
			if s != "" && !uuid.IsValid(s) {
				return ValidationError{Message: key + " must be a valid UUID"}
			}

			return nil
		})
	}

	// 5. URL Validation
	if b.IsURL {
		validators = append(validators, func(s string) error {
			if s == "" {
				return nil
			}

			u, err := url.ParseRequestURI(s)
			if err != nil || u.Scheme == "" || u.Host == "" {
				return ValidationError{Message: key + " must be a valid absolute URL"}
			}

			return nil
		})
	}

	// 6. Enum Values
	if len(b.EnumVals) > 0 {
		enums := slices.Clone(b.EnumVals)
		enumListStr := strings.Join(b.EnumVals, ", ")

		validators = append(validators, func(s string) error {
			if s == "" {
				return nil
			}
			for _, e := range enums {
				if bytesconv.EqualFoldASCII(s, e) {
					return nil
				}
			}

			return ValidationError{Message: fmt.Sprintf("%s must be one of [%s], got %q", key, enumListStr, s)}
		})
	}

	targetKind := b.Kind
	if b.IsPtr {
		targetKind = b.ElemKind
	}

	// 7. String / Numeric Min & Max
	if b.HasMin {
		minVal := b.MinVal
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

				if num, err := strconv.ParseFloat(s, 64); err == nil && num < minVal {
					return ValidationError{Message: fmt.Sprintf("%s value must be at least %v", key, minVal)}
				}

				return nil
			})
		}
	}

	if b.HasMax {
		maxVal := b.MaxVal
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

				if num, err := strconv.ParseFloat(s, 64); err == nil && num > maxVal {
					return ValidationError{Message: fmt.Sprintf("%s value must be at most %v", key, maxVal)}
				}

				return nil
			})
		}
	}

	// 8. Mathematical Numeric Boundaries (gt, ge, lt, le, multiple_of)
	if refkit.IsNumeric(targetKind) {
		if b.HasGT {
			gtVal := b.GTVal

			validators = append(validators, func(s string) error {
				if s == "" {
					return nil
				}

				if num, err := strconv.ParseFloat(s, 64); err == nil && num <= gtVal {
					return ValidationError{Message: fmt.Sprintf("%s must be greater than %v", key, gtVal)}
				}

				return nil
			})
		}

		if b.HasGE {
			geVal := b.GEVal

			validators = append(validators, func(s string) error {
				if s == "" {
					return nil
				}

				if num, err := strconv.ParseFloat(s, 64); err == nil && num < geVal {
					return ValidationError{Message: fmt.Sprintf("%s must be greater than or equal to %v", key, geVal)}
				}

				return nil
			})
		}

		if b.HasLT {
			ltVal := b.LTVal

			validators = append(validators, func(s string) error {
				if s == "" {
					return nil
				}

				if num, err := strconv.ParseFloat(s, 64); err == nil && num >= ltVal {
					return ValidationError{Message: fmt.Sprintf("%s must be less than %v", key, ltVal)}
				}

				return nil
			})
		}

		if b.HasLE {
			leVal := b.LEVal

			validators = append(validators, func(s string) error {
				if s == "" {
					return nil
				}

				if num, err := strconv.ParseFloat(s, 64); err == nil && num > leVal {
					return ValidationError{Message: fmt.Sprintf("%s must be less than or equal to %v", key, leVal)}
				}

				return nil
			})
		}

		if b.HasMultipleOf && b.MultipleOfVal > 0 {
			mVal := b.MultipleOfVal

			validators = append(validators, func(s string) error {
				if s == "" {
					return nil
				}

				if num, err := strconv.ParseFloat(s, 64); err == nil {
					if math.Mod(num, mVal) != 0 {
						return ValidationError{Message: fmt.Sprintf("%s must be a multiple of %v", key, mVal)}
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
