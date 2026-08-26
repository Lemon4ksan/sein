// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

// AssignSlice parses and allocates a slice of scalar values.
func AssignSlice(req RequestView, fieldPtr unsafe.Pointer, b *FieldBinding, initialVal string) error {
	var vals []string
	if b.Source == SourceQuery {
		q := req.RawURLQuery()
		if sliceVals, ok := q[b.Key]; ok && len(sliceVals) > 0 {
			for _, sv := range sliceVals {
				for _, item := range strings.Split(sv, ",") {
					if item = strings.TrimSpace(item); item != "" {
						vals = append(vals, item)
					}
				}
			}
		}
	}

	if len(vals) == 0 && initialVal != "" {
		for _, item := range strings.Split(initialVal, ",") {
			if item = strings.TrimSpace(item); item != "" {
				vals = append(vals, item)
			}
		}
	}

	if b.HasMin && float64(len(vals)) < b.MinVal {
		return ValidationError{Message: fmt.Sprintf("%s slice length must be at least %v", b.Key, b.MinVal)}
	}
	if b.HasMax && float64(len(vals)) > b.MaxVal {
		return ValidationError{Message: fmt.Sprintf("%s slice length must be at most %v", b.Key, b.MaxVal)}
	}

	sliceVal := reflect.MakeSlice(b.FieldType, len(vals), len(vals))
	for i, v := range vals {
		v = ApplyTransforms(v, b)
		elemPtr := sliceVal.Index(i).Addr().UnsafePointer()
		elemType := b.FieldType.Elem()
		if err := AssignScalar(elemPtr, b.SliceElemKind, elemType, v, b.Source, b.Key); err != nil {
			return err
		}
	}

	reflect.NewAt(b.FieldType, fieldPtr).Elem().Set(sliceVal)
	return nil
}
