// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"fmt"
	"net"
	"reflect"
	"strings"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/refkit"
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
	Format        string
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

// StructDescriptor caches the precompiled pipeline steps and layout of a struct type.
type StructDescriptor struct {
	HasBodyFields bool
	PathKeys      generic.Set[string]
	Steps         []FieldStep
}

var (
	descriptorCache generic.ConcurrentMap[reflect.Type, *StructDescriptor]
	netIPType       = reflect.TypeFor[net.IP]()
	bytesSliceType  = reflect.TypeFor[[]byte]()

	// Declarative tag sources registry
	tagSources = []struct {
		tag    string
		source ParamSource
	}{
		{"path", SourcePath},
		{"param", SourcePath},
		{"query", SourceQuery},
		{"header", SourceHeader},
		{"cookie", SourceCookie},
		{"auth", SourceAuth},
		{"net", SourceNet},
		{"form", SourceForm},
		{"file", SourceFile},
		{"files", SourceFiles},
		{"body", SourceBodyRaw},
		{"ctx", SourceContext},
		{"context", SourceContext},
	}
)

func populateFieldBinding(tag refkit.Tag, b *FieldBinding) {
	b.Key = tag.Name
	b.Required = tag.Has("required")
	b.DefaultValue = tag.Get("default")
	b.Format = tag.Get("format")
	b.Trim = tag.Has("trim")
	b.Lower = tag.Has("lower")
	b.Upper = tag.Has("upper")
	b.SingleSpace = tag.Has("single_space") || tag.Has("squish")
	b.DigitsOnly = tag.Has("digits_only")
	b.IsEmail = tag.Has("email")
	b.MinVal, b.HasMin = tag.GetFloat("min")
	b.MaxVal, b.HasMax = tag.GetFloat("max")
	b.LenVal, b.HasLen = tag.GetInt("len")
	b.EnumVals = tag.SplitOption("enum", "|")
}

// GetDescriptor retrieves or compiles a cached StructDescriptor for typ.
func GetDescriptor(typ reflect.Type) *StructDescriptor {
	typ = refkit.DerefType(typ)
	if typ.Kind() != reflect.Struct {
		return nil
	}

	if d, ok := descriptorCache.Load(typ); ok {
		return d
	}

	desc := &StructDescriptor{
		PathKeys: generic.NewSet[string](),
	}

	for field := range typ.Fields() {
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

		// Check declarative tag sources in order
		for _, ts := range tagSources {
			if raw, ok := field.Tag.Lookup(ts.tag); ok {
				tag := refkit.ParseTag(raw)
				b.Source = ts.source
				populateFieldBinding(tag, &b)

				defaultName := field.Name
				switch b.Source {
				case SourceAuth:
					defaultName = "bearer"
				case SourceNet:
					defaultName = "ip"
				}
				b.Key = generic.Coalesce(b.Key, defaultName)

				switch b.Source {
				case SourcePath:
					b.Required = true
					desc.PathKeys.Add(b.Key)
				case SourceBodyRaw:
					if b.Key != "raw" && b.FieldType != bytesSliceType {
						b.Source = SourceBodyString
					}
				}

				matched = true
				break
			}
		}

		if matched {
			step := CompileFieldStep(&b)
			desc.Steps = append(desc.Steps, step)
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
	typ = refkit.DerefType(typ)
	if typ.Kind() != reflect.Struct {
		return
	}

	var pathVars []string
	for seg := range strings.SplitSeq(routePath, "/") {
		if after, ok := strings.CutPrefix(seg, ":"); ok {
			pathVars = append(pathVars, after)
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
		if !desc.PathKeys.Has(pv) {
			panic(fmt.Sprintf("sein: route %q declares path param :%s, but DTO %s has no matching field with `path:%q` tag",
				routePath, pv, typ.Name(), pv))
		}
	}
}
