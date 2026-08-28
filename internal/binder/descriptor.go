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
	Separator     string
	Pattern       string
	IsPtr         bool
	ElemKind      reflect.Kind
	IsSlice       bool
	SliceElemKind reflect.Kind

	// String Sanitizers
	Signed      bool
	Trim        bool
	Lower       bool
	Upper       bool
	SingleSpace bool
	DigitsOnly  bool

	// Specialized Type Validators / Decoders
	IsEmail  bool
	IsUUID   bool
	IsURL    bool
	IsBase64 bool
	IsHex    bool

	// Declarative Constraints
	HasMin        bool
	MinVal        float64
	HasMax        bool
	MaxVal        float64
	HasGT         bool
	GTVal         float64
	HasGE         bool
	GEVal         float64
	HasLT         bool
	LTVal         float64
	HasLE         bool
	LEVal         float64
	HasMultipleOf bool
	MultipleOfVal float64
	HasLen        bool
	LenVal        int
	EnumVals      []string
}

// HasValidationRules reports whether the field has declarative validation constraints.
func (b *FieldBinding) HasValidationRules() bool {
	return b.Required || b.HasMin || b.HasMax || b.HasLen || b.HasGT || b.HasGE ||
		b.HasLT || b.HasLE || b.HasMultipleOf || b.Pattern != "" || b.IsEmail ||
		b.IsUUID || b.IsURL || b.IsBase64 || b.IsHex || len(b.EnumVals) > 0
}

// HasSanitizationRules reports whether the field has string sanitization transforms.
func (b *FieldBinding) HasSanitizationRules() bool {
	return b.Trim || b.Lower || b.Upper || b.SingleSpace || b.DigitsOnly
}

// StructDescriptor caches the precompiled pipeline steps and layout of a struct type.
type StructDescriptor struct {
	HasBodyFields  bool
	PathKeys       generic.Set[string]
	Steps          []FieldStep
	PostValidators []PostValidatorFunc
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

func populateValidationRules(tag refkit.Tag, b *FieldBinding) {
	if tag.Name == "required" || tag.Has("required") {
		b.Required = true
	}
	if tag.Name == "email" || tag.Has("email") {
		b.IsEmail = true
	}
	if tag.Name == "uuid" || tag.Has("uuid") {
		b.IsUUID = true
	}
	if tag.Name == "url" || tag.Has("url") {
		b.IsURL = true
	}
	if tag.Name == "base64" || tag.Has("base64") {
		b.IsBase64 = true
	}
	if tag.Name == "hex" || tag.Has("hex") {
		b.IsHex = true
	}
	if tag.Name == "sign" || tag.Name == "signed" || tag.Has("sign") || tag.Has("signed") {
		b.Signed = true
	}

	if v, ok := tag.GetFloat("min"); ok {
		b.HasMin = true
		b.MinVal = v
	} else if strings.HasPrefix(tag.Name, "min=") {
		if val, err := strconv.ParseFloat(strings.TrimPrefix(tag.Name, "min="), 64); err == nil {
			b.HasMin = true
			b.MinVal = val
		}
	}

	if v, ok := tag.GetFloat("max"); ok {
		b.HasMax = true
		b.MaxVal = v
	} else if strings.HasPrefix(tag.Name, "max=") {
		if val, err := strconv.ParseFloat(strings.TrimPrefix(tag.Name, "max="), 64); err == nil {
			b.HasMax = true
			b.MaxVal = val
		}
	}

	if v, ok := tag.GetInt("len"); ok {
		b.HasLen = true
		b.LenVal = v
	} else if strings.HasPrefix(tag.Name, "len=") {
		if val, err := strconv.Atoi(strings.TrimPrefix(tag.Name, "len=")); err == nil {
			b.HasLen = true
			b.LenVal = val
		}
	}

	if enums := tag.SplitOption("enum", "|"); len(enums) > 0 {
		b.EnumVals = enums
	} else if strings.HasPrefix(tag.Name, "enum=") {
		b.EnumVals = strings.Split(strings.TrimPrefix(tag.Name, "enum="), "|")
	}

	if v, ok := tag.GetFloat("gt"); ok {
		b.HasGT = true
		b.GTVal = v
	}
	if v, ok := tag.GetFloat("ge"); ok {
		b.HasGE = true
		b.GEVal = v
	}
	if v, ok := tag.GetFloat("lt"); ok {
		b.HasLT = true
		b.LTVal = v
	}
	if v, ok := tag.GetFloat("le"); ok {
		b.HasLE = true
		b.LEVal = v
	}
	if tag.Name == "positive" || tag.Has("positive") {
		b.HasGT = true
		b.GTVal = 0
	}
	if tag.Name == "negative" || tag.Has("negative") {
		b.HasLT = true
		b.LTVal = 0
	}
	if tag.Name == "non_negative" || tag.Has("non_negative") {
		b.HasGE = true
		b.GEVal = 0
	}
	if v, ok := tag.GetFloat("multiple_of"); ok {
		b.HasMultipleOf = true
		b.MultipleOfVal = v
	}
	if pat := tag.Get("pattern"); pat != "" {
		b.Pattern = pat
	} else if strings.HasPrefix(tag.Name, "pattern=") {
		b.Pattern = strings.TrimPrefix(tag.Name, "pattern=")
	}
	if fmtVal := tag.Get("format"); fmtVal != "" {
		b.Format = fmtVal
	}
}

func populateSanitizationRules(tag refkit.Tag, b *FieldBinding) {
	if tag.Name == "trim" || tag.Has("trim") {
		b.Trim = true
	}
	if tag.Name == "lower" || tag.Has("lower") {
		b.Lower = true
	}
	if tag.Name == "upper" || tag.Has("upper") {
		b.Upper = true
	}
	if tag.Name == "single_space" || tag.Name == "squish" || tag.Has("single_space") || tag.Has("squish") {
		b.SingleSpace = true
	}
	if tag.Name == "digits_only" || tag.Has("digits_only") {
		b.DigitsOnly = true
	}
}

func populateFieldBinding(tag refkit.Tag, b *FieldBinding) {
	if b.Key == "" && tag.Name != "" {
		b.Key = tag.Name
	}
	if def := tag.Get("default"); def != "" {
		b.DefaultValue = def
	}
	if sep := tag.Get("sep"); sep != "" {
		b.Separator = sep
	} else if b.Separator == "" {
		b.Separator = ","
	}
	populateValidationRules(tag, b)
	populateSanitizationRules(tag, b)
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
				case SourceForm:
					if b.FieldType.String() == "*sein.File" || (isPtr && ft.Elem().Name() == "File") {
						b.Source = SourceFile
					} else if b.FieldType.String() == "[]*sein.File" || (isSlice && ft.Elem().Kind() == reflect.Pointer && ft.Elem().Elem().Name() == "File") {
						b.Source = SourceFiles
					}

				case SourceBodyRaw:
					if b.Key != "raw" && b.FieldType != bytesSliceType {
						b.Source = SourceBodyString
					}
				}

				matched = true

				break
			}
		}

		// Check dedicated validate, sanitize, and binding tags on all fields
		if raw, ok := field.Tag.Lookup("validate"); ok {
			tag := refkit.ParseTag(raw)
			populateFieldBinding(tag, &b)
		}
		if raw, ok := field.Tag.Lookup("sanitize"); ok {
			tag := refkit.ParseTag(raw)
			populateFieldBinding(tag, &b)
		}
		if raw, ok := field.Tag.Lookup("binding"); ok {
			tag := refkit.ParseTag(raw)
			populateFieldBinding(tag, &b)
		}

		if matched {
			step := CompileFieldStep(&b)
			desc.Steps = append(desc.Steps, step)
		} else {
			if jsonTag, ok := field.Tag.Lookup("json"); ok {
				desc.HasBodyFields = true
				jsonName := strings.Split(jsonTag, ",")[0]
				if jsonName != "" && jsonName != "-" {
					b.Key = generic.Coalesce(b.Key, jsonName)
				}
			}
			if b.HasValidationRules() || b.HasSanitizationRules() {
				b.Key = generic.Coalesce(b.Key, field.Name)
				postStep := CompilePostValidator(&b)
				desc.PostValidators = append(desc.PostValidators, postStep)
			}
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
		panic(
			fmt.Sprintf(
				"sein: route %q declares path params %v, but DTO %s is not a struct",
				routePath,
				pathVars,
				typ.Name(),
			),
		)
	}

	for _, pv := range pathVars {
		if !desc.PathKeys.Has(pv) {
			panic(
				fmt.Sprintf(
					"sein: route %q declares path param :%s, but DTO %s has no matching field with `path:%q` tag",
					routePath,
					pv,
					typ.Name(),
					pv,
				),
			)
		}
	}
}
