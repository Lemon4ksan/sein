// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// ParamSource defines where a DTO field value originates from.
type ParamSource uint8

const (
	SourcePath ParamSource = iota
	SourceQuery
	SourceHeader
	SourceCookie
	SourceAuth
	SourceNet
	SourceForm
	SourceFile
	SourceFiles
	SourceBodyRaw
	SourceBodyString
	SourceContext
)

// FieldBinding contains parsed metadata and flags for a struct field.
type FieldBinding struct {
	Offset        uintptr
	Kind          reflect.Kind
	FieldType     reflect.Type
	Source        ParamSource
	Key           string
	Required      bool
	DefaultValue  string
	IsPtr         bool
	ElemKind      reflect.Kind
	IsSlice       bool
	SliceElemKind reflect.Kind

	// String Sanitizers
	Trim        bool
	Lower       bool
	Upper       bool
	SingleSpace bool
	DigitsOnly  bool

	// Declarative Constraints
	HasMin   bool
	MinVal   float64
	HasMax   bool
	MaxVal   float64
	HasLen   bool
	LenVal   int
	EnumVals []string
	IsEmail  bool
}

// StructDescriptor caches the precompiled pipeline stages and layout of a struct type.
type StructDescriptor struct {
	HasBodyFields bool
	PathKeys      map[string]bool
	Pipelines     []FieldPipeline
}

var (
	descriptorCache sync.Map
	netIPType       = reflect.TypeFor[net.IP]()
	bytesSliceType  = reflect.TypeFor[[]byte]()
)

// ParseTagOptions extracts key, options, sanitizers, and validation flags from a struct tag.
func ParseTagOptions(tagStr string, binding *FieldBinding) {
	parts := strings.Split(tagStr, ",")
	binding.Key = strings.TrimSpace(parts[0])

	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		switch {
		case part == "required":
			binding.Required = true
		case strings.HasPrefix(part, "default="):
			binding.DefaultValue = strings.TrimPrefix(part, "default=")
		case part == "trim":
			binding.Trim = true
		case part == "lower":
			binding.Lower = true
		case part == "upper":
			binding.Upper = true
		case part == "single_space" || part == "squish":
			binding.SingleSpace = true
		case part == "digits_only":
			binding.DigitsOnly = true
		case part == "email":
			binding.IsEmail = true
		case strings.HasPrefix(part, "min="):
			if v, err := strconv.ParseFloat(strings.TrimPrefix(part, "min="), 64); err == nil {
				binding.HasMin = true
				binding.MinVal = v
			}
		case strings.HasPrefix(part, "max="):
			if v, err := strconv.ParseFloat(strings.TrimPrefix(part, "max="), 64); err == nil {
				binding.HasMax = true
				binding.MaxVal = v
			}
		case strings.HasPrefix(part, "len="):
			if v, err := strconv.Atoi(strings.TrimPrefix(part, "len=")); err == nil {
				binding.HasLen = true
				binding.LenVal = v
			}
		case strings.HasPrefix(part, "enum="):
			binding.EnumVals = strings.Split(strings.TrimPrefix(part, "enum="), "|")
		}
	}
}

// GetDescriptor retrieves or compiles a cached StructDescriptor for typ.
func GetDescriptor(typ reflect.Type) *StructDescriptor {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}

	if d, ok := descriptorCache.Load(typ); ok {
		return d.(*StructDescriptor)
	}

	desc := &StructDescriptor{
		PathKeys: make(map[string]bool),
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		ft := field.Type
		isPtr := ft.Kind() == reflect.Pointer
		elemKind := ft.Kind()
		if isPtr {
			elemKind = ft.Elem().Kind()
		}

		isSlice := ft.Kind() == reflect.Slice && ft != netIPType && ft != bytesSliceType
		sliceElemKind := reflect.Invalid
		if isSlice {
			sliceElemKind = ft.Elem().Kind()
		}

		b := FieldBinding{
			Offset:        field.Offset,
			Kind:          field.Type.Kind(),
			FieldType:     field.Type,
			IsPtr:         isPtr,
			ElemKind:      elemKind,
			IsSlice:       isSlice,
			SliceElemKind: sliceElemKind,
		}

		var matched bool

		if tag, ok := field.Tag.Lookup("path"); ok {
			b.Source = SourcePath
			ParseTagOptions(tag, &b)
			if b.Key == "" {
				b.Key = field.Name
			}
			b.Required = true
			matched = true
			desc.PathKeys[b.Key] = true
		} else if tag, ok := field.Tag.Lookup("param"); ok {
			b.Source = SourcePath
			ParseTagOptions(tag, &b)
			if b.Key == "" {
				b.Key = field.Name
			}
			b.Required = true
			matched = true
			desc.PathKeys[b.Key] = true
		} else if tag, ok := field.Tag.Lookup("query"); ok {
			b.Source = SourceQuery
			ParseTagOptions(tag, &b)
			if b.Key == "" {
				b.Key = field.Name
			}
			matched = true
		} else if tag, ok := field.Tag.Lookup("header"); ok {
			b.Source = SourceHeader
			ParseTagOptions(tag, &b)
			if b.Key == "" {
				b.Key = field.Name
			}
			matched = true
		} else if tag, ok := field.Tag.Lookup("cookie"); ok {
			b.Source = SourceCookie
			ParseTagOptions(tag, &b)
			if b.Key == "" {
				b.Key = field.Name
			}
			matched = true
		} else if tag, ok := field.Tag.Lookup("auth"); ok {
			b.Source = SourceAuth
			ParseTagOptions(tag, &b)
			if b.Key == "" {
				b.Key = "bearer"
			}
			matched = true
		} else if tag, ok := field.Tag.Lookup("net"); ok {
			b.Source = SourceNet
			ParseTagOptions(tag, &b)
			if b.Key == "" {
				b.Key = "ip"
			}
			matched = true
		} else if tag, ok := field.Tag.Lookup("form"); ok {
			b.Source = SourceForm
			ParseTagOptions(tag, &b)
			if b.Key == "" {
				b.Key = field.Name
			}
			matched = true
		} else if tag, ok := field.Tag.Lookup("file"); ok {
			b.Source = SourceFile
			ParseTagOptions(tag, &b)
			if b.Key == "" {
				b.Key = field.Name
			}
			matched = true
		} else if tag, ok := field.Tag.Lookup("files"); ok {
			b.Source = SourceFiles
			ParseTagOptions(tag, &b)
			if b.Key == "" {
				b.Key = field.Name
			}
			matched = true
		} else if tag, ok := field.Tag.Lookup("body"); ok {
			ParseTagOptions(tag, &b)
			if b.Key == "raw" || b.FieldType == bytesSliceType {
				b.Source = SourceBodyRaw
			} else {
				b.Source = SourceBodyString
			}
			matched = true
		} else if tag, ok := field.Tag.Lookup("ctx"); ok {
			b.Source = SourceContext
			ParseTagOptions(tag, &b)
			matched = true
		} else if tag, ok := field.Tag.Lookup("context"); ok {
			b.Source = SourceContext
			ParseTagOptions(tag, &b)
			matched = true
		}

		if matched {
			extractor, specialExtractor := CompileExtractor(b.Source, b.Key, b.Required, b.DefaultValue, b.FieldType, b.IsSlice)
			pipeline := FieldPipeline{
				Offset:         b.Offset,
				Extract:        extractor,
				SpecialExtract: specialExtractor,
				Transforms:     CompileTransforms(&b),
				Validators:     CompileValidators(&b),
				Assign:         CompileAssigner(&b),
			}
			desc.Pipelines = append(desc.Pipelines, pipeline)
		}

		if _, ok := field.Tag.Lookup("json"); ok {
			desc.HasBodyFields = true
		}
	}

	descriptorCache.Store(typ, desc)
	return desc
}

// ValidateRouteBinding checks at startup that all path variables in routePath have matching fields in typ.
func ValidateRouteBinding(typ reflect.Type, routePath string) {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return
	}

	var pathVars []string
	for _, seg := range strings.Split(routePath, "/") {
		if strings.HasPrefix(seg, ":") {
			pathVars = append(pathVars, strings.TrimPrefix(seg, ":"))
		}
	}

	if len(pathVars) == 0 {
		return
	}

	desc := GetDescriptor(typ)
	if desc == nil {
		panic(fmt.Sprintf("sein: route %q declares path params %v, but DTO %s is not a struct", routePath, pathVars, typ.Name()))
	}

	for _, pv := range pathVars {
		if !desc.PathKeys[pv] {
			panic(fmt.Sprintf("sein: route %q declares path param :%s, but DTO %s has no matching field with `path:%q` tag",
				routePath, pv, typ.Name(), pv))
		}
	}
}
