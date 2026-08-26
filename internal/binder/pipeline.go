// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"unsafe"
)

// ExtractorFunc extracts raw data from a request.
type ExtractorFunc func(req RequestView) (raw string, present bool, err error)

// SpecialExtractorFunc directly handles non-string sources like files or context injection.
type SpecialExtractorFunc func(req RequestView, fieldPtr unsafe.Pointer) error

// TransformFunc applies a transformation to an extracted string value.
type TransformFunc func(s string) string

// ValidatorFunc validates a value against declarative constraints.
type ValidatorFunc func(s string) error

// AssignerFunc parses and writes the transformed value into the target field memory pointer.
type AssignerFunc func(req RequestView, fieldPtr unsafe.Pointer, raw string) error

// FieldPipeline encapsulates the precompiled 4-stage pipeline for a single DTO struct field.
type FieldPipeline struct {
	Offset           uintptr
	Extract          ExtractorFunc
	SpecialExtract   SpecialExtractorFunc
	Transforms       []TransformFunc
	Validators       []ValidatorFunc
	Assign           AssignerFunc
}

// Execute runs the precompiled 4-stage pipeline for this field against the destination struct pointer.
func (p *FieldPipeline) Execute(req RequestView, structPtr unsafe.Pointer) error {
	fieldPtr := unsafe.Pointer(uintptr(structPtr) + p.Offset)

	// Special extractors (files, raw body, context injection) bypass string parsing
	if p.SpecialExtract != nil {
		return p.SpecialExtract(req, fieldPtr)
	}

	// 1. Extract Stage
	raw, present, err := p.Extract(req)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}

	// 2. Transform Stage
	for _, t := range p.Transforms {
		raw = t(raw)
	}

	// 3. Validate Stage
	for _, v := range p.Validators {
		if err := v(raw); err != nil {
			return err
		}
	}

	// 4. Assign Stage
	return p.Assign(req, fieldPtr, raw)
}
