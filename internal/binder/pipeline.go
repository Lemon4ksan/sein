// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unsafe"

	"github.com/lemon4ksan/foundation/generic"
)

// CompileFieldStep compiles the complete 4-stage pipeline (extraction, transforms, validations, typed assignment)
// into a unified, non-branching FieldStep closure executed at runtime.
func CompileFieldStep(b *FieldBinding) FieldStep {
	if special := compileSpecialStep(b, b.Offset); special != nil {
		return special
	}

	extractor := compileStringExtractor(b)
	transforms := CompileTransforms(b)
	validators := CompileValidators(b)

	switch {
	case b.IsSlice:
		return compileSliceStep(b, extractor, transforms)
	case b.IsPtr:
		return compilePointerStep(b, extractor, transforms, validators)
	default:
		return compileScalarStep(b, extractor, transforms, validators)
	}
}

func compileScalarStep(
	b *FieldBinding,
	extract StringExtractorFunc,
	transforms []TransformFunc,
	validators []ValidatorFunc,
) FieldStep {
	setter := CompileSetter(b.FieldType, b.Kind, b.Source, b.Key, b.Format, b.IsHex, b.IsBase64)
	offset := b.Offset

	return func(req RequestView, structPtr unsafe.Pointer) error {
		opt, err := extract(req)
		if err != nil || !opt.IsPresent() {
			return err
		}

		raw, err := processRaw(opt.MustValue(), transforms, validators)
		if err != nil {
			return err
		}

		return setter(unsafe.Add(structPtr, offset), raw)
	}
}

func compilePointerStep(
	b *FieldBinding,
	extract StringExtractorFunc,
	transforms []TransformFunc,
	validators []ValidatorFunc,
) FieldStep {
	elemType := b.FieldType.Elem()
	setter := CompileSetter(elemType, b.ElemKind, b.Source, b.Key, b.Format, b.IsHex, b.IsBase64)
	offset := b.Offset

	return func(req RequestView, structPtr unsafe.Pointer) error {
		opt, err := extract(req)
		if err != nil || !opt.IsPresent() {
			return err
		}

		raw, err := processRaw(opt.MustValue(), transforms, validators)
		if err != nil {
			return err
		}

		fieldPtr := unsafe.Add(structPtr, offset)
		valPtr := reflect.New(elemType).UnsafePointer()
		*(*unsafe.Pointer)(fieldPtr) = valPtr

		return setter(valPtr, raw)
	}
}

func compileSliceStep(b *FieldBinding, extract StringExtractorFunc, transforms []TransformFunc) FieldStep {
	elemSetter := CompileSetter(b.FieldType.Elem(), b.SliceElemKind, b.Source, b.Key, b.Format, false, false)
	sliceType := b.FieldType
	key := b.Key
	source := b.Source
	sep := b.Separator
	hasMin, minVal := b.HasMin, b.MinVal
	hasMax, maxVal := b.HasMax, b.MaxVal
	offset := b.Offset

	return func(req RequestView, structPtr unsafe.Pointer) error {
		opt, err := extract(req)
		if err != nil || !opt.IsPresent() {
			return err
		}

		vals := extractSliceValues(req, source, key, opt.MustValue(), sep)
		if hasMin && float64(len(vals)) < minVal {
			return ValidationError{Message: fmt.Sprintf("%s slice length must be at least %v", key, minVal)}
		}

		if hasMax && float64(len(vals)) > maxVal {
			return ValidationError{Message: fmt.Sprintf("%s slice length must be at most %v", key, maxVal)}
		}

		sliceVal := reflect.MakeSlice(sliceType, len(vals), len(vals))
		for i, v := range vals {
			for _, t := range transforms {
				v = t(v)
			}

			elemPtr := sliceVal.Index(i).Addr().UnsafePointer()
			if err := elemSetter(elemPtr, v); err != nil {
				return err
			}
		}

		reflect.NewAt(sliceType, unsafe.Add(structPtr, offset)).Elem().Set(sliceVal)

		return nil
	}
}

func processRaw(raw string, transforms []TransformFunc, validators []ValidatorFunc) (string, error) {
	for _, t := range transforms {
		raw = t(raw)
	}

	for _, v := range validators {
		if err := v(raw); err != nil {
			return "", err
		}
	}

	return raw, nil
}

func extractSliceValues(req RequestView, source ParamSource, key, raw, sep string) []string {
	var vals []string
	if source == SourceQuery {
		for _, sv := range req.RawURLQuery()[key] {
			for item := range strings.SplitSeq(sv, sep) {
				if item = strings.TrimSpace(item); item != "" {
					vals = append(vals, item)
				}
			}
		}
	}

	if len(vals) == 0 && raw != "" {
		for item := range strings.SplitSeq(raw, sep) {
			if item = strings.TrimSpace(item); item != "" {
				vals = append(vals, item)
			}
		}
	}

	return generic.Map(vals, strings.TrimSpace)
}

// CompilePostValidator compiles in-place validation and sanitization for struct fields (e.g. JSON body fields).
func CompilePostValidator(b *FieldBinding) PostValidatorFunc {
	transforms := CompileTransforms(b)
	validators := CompileValidators(b)
	offset := b.Offset
	key := b.Key
	required := b.Required
	kind := b.Kind
	if b.IsPtr {
		kind = b.ElemKind
	}

	return func(structPtr unsafe.Pointer) error {
		fieldPtr := unsafe.Add(structPtr, offset)

		if b.IsPtr {
			valPtr := *(*unsafe.Pointer)(fieldPtr)
			if valPtr == nil {
				if required {
					return ValidationError{Message: key + " is required"}
				}
				return nil
			}
			fieldPtr = valPtr
		}

		switch kind {
		case reflect.String:
			strPtr := (*string)(fieldPtr)
			val := *strPtr
			if required && len(val) == 0 {
				return ValidationError{Message: key + " is required"}
			}
			for _, t := range transforms {
				val = t(val)
			}
			*strPtr = val
			for _, v := range validators {
				if err := v(val); err != nil {
					return err
				}
			}

		case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
			var intVal int64
			switch kind {
			case reflect.Int:
				intVal = int64(*(*int)(fieldPtr))
			case reflect.Int64:
				intVal = *(*int64)(fieldPtr)
			case reflect.Int32:
				intVal = int64(*(*int32)(fieldPtr))
			case reflect.Int16:
				intVal = int64(*(*int16)(fieldPtr))
			case reflect.Int8:
				intVal = int64(*(*int8)(fieldPtr))
			}
			if required && intVal == 0 {
				return ValidationError{Message: key + " is required"}
			}
			str := strconv.FormatInt(intVal, 10)
			for _, v := range validators {
				if err := v(str); err != nil {
					return err
				}
			}

		case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
			var uintVal uint64
			switch kind {
			case reflect.Uint:
				uintVal = uint64(*(*uint)(fieldPtr))
			case reflect.Uint64:
				uintVal = *(*uint64)(fieldPtr)
			case reflect.Uint32:
				uintVal = uint64(*(*uint32)(fieldPtr))
			case reflect.Uint16:
				uintVal = uint64(*(*uint16)(fieldPtr))
			case reflect.Uint8:
				uintVal = uint64(*(*uint8)(fieldPtr))
			}
			if required && uintVal == 0 {
				return ValidationError{Message: key + " is required"}
			}
			str := strconv.FormatUint(uintVal, 10)
			for _, v := range validators {
				if err := v(str); err != nil {
					return err
				}
			}

		case reflect.Float64, reflect.Float32:
			var floatVal float64
			if kind == reflect.Float64 {
				floatVal = *(*float64)(fieldPtr)
			} else {
				floatVal = float64(*(*float32)(fieldPtr))
			}
			if required && floatVal == 0 {
				return ValidationError{Message: key + " is required"}
			}
			str := strconv.FormatFloat(floatVal, 'f', -1, 64)
			for _, v := range validators {
				if err := v(str); err != nil {
					return err
				}
			}

		case reflect.Slice:
			sliceVal := reflect.NewAt(b.FieldType, fieldPtr).Elem()
			l := sliceVal.Len()
			if required && l == 0 {
				return ValidationError{Message: key + " is required"}
			}
			if b.HasMin && float64(l) < b.MinVal {
				return ValidationError{Message: fmt.Sprintf("%s slice length must be at least %v", key, b.MinVal)}
			}
			if b.HasMax && float64(l) > b.MaxVal {
				return ValidationError{Message: fmt.Sprintf("%s slice length must be at most %v", key, b.MaxVal)}
			}
			if b.HasLen && l != b.LenVal {
				return ValidationError{Message: fmt.Sprintf("%s length must be exactly %d", key, b.LenVal)}
			}

		default:
			if b.FieldType.String() == "sein.Secret[string]" || (b.Kind == reflect.Struct && b.FieldType.Name() == "Secret") {
				strPtr := (*string)(fieldPtr)
				val := *strPtr
				if required && len(val) == 0 {
					return ValidationError{Message: key + " is required"}
				}
				for _, t := range transforms {
					val = t(val)
				}
				*strPtr = val
				for _, v := range validators {
					if err := v(val); err != nil {
						return err
					}
				}
			}
		}

		return nil
	}
}
