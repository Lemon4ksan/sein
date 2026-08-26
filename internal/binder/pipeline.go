// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"fmt"
	"reflect"
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

func compileScalarStep(b *FieldBinding, extract StringExtractorFunc, transforms []TransformFunc, validators []ValidatorFunc) FieldStep {
	setter := CompileSetter(b.FieldType, b.Kind, b.Source, b.Key, b.Format)
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

func compilePointerStep(b *FieldBinding, extract StringExtractorFunc, transforms []TransformFunc, validators []ValidatorFunc) FieldStep {
	elemType := b.FieldType.Elem()
	setter := CompileSetter(elemType, b.ElemKind, b.Source, b.Key, b.Format)
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
	elemSetter := CompileSetter(b.FieldType.Elem(), b.SliceElemKind, b.Source, b.Key, b.Format)
	sliceType := b.FieldType
	key := b.Key
	source := b.Source
	hasMin, minVal := b.HasMin, b.MinVal
	hasMax, maxVal := b.HasMax, b.MaxVal
	offset := b.Offset

	return func(req RequestView, structPtr unsafe.Pointer) error {
		opt, err := extract(req)
		if err != nil || !opt.IsPresent() {
			return err
		}

		vals := extractSliceValues(req, source, key, opt.MustValue())
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

func extractSliceValues(req RequestView, source ParamSource, key, raw string) []string {
	var vals []string
	if source == SourceQuery {
		for _, sv := range req.RawURLQuery()[key] {
			for _, item := range strings.Split(sv, ",") {
				if item = strings.TrimSpace(item); item != "" {
					vals = append(vals, item)
				}
			}
		}
	}
	if len(vals) == 0 && raw != "" {
		for _, item := range strings.Split(raw, ",") {
			if item = strings.TrimSpace(item); item != "" {
				vals = append(vals, item)
			}
		}
	}
	return generic.Map(vals, strings.TrimSpace)
}
