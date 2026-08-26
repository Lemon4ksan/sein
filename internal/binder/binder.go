// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

// Common error sentinels returned by binder engine.
var (
	ErrMissingPathParam   = errors.New("required path parameter is missing")
	ErrMissingQueryParam  = errors.New("required query parameter is missing")
	ErrMissingHeader      = errors.New("required header is missing")
	ErrMissingCookie      = errors.New("required cookie is missing")
	ErrMissingBearerToken = errors.New("authorization bearer token is required")
	ErrMissingContext     = errors.New("required context value is missing")
	ErrEmptyRequestBody   = errors.New("request body cannot be empty")
)

// Ingest extracts multi-source request data into dest pointer.
func Ingest(req RequestView, dest any) error {
	// 1. Check Ingestable interface (vortex gen fast-path)
	if ing, ok := dest.(Ingestable); ok {
		if err := ing.IngestAny(req); err != nil {
			return err
		}
		return RunValidation(dest)
	}

	val := reflect.ValueOf(dest)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return errors.New("dest must be a non-nil pointer to struct")
	}

	typ := val.Type().Elem()
	desc := GetDescriptor(typ)
	if desc == nil {
		if len(req.Body()) > 0 {
			if err := req.BindJSON(dest); err != nil {
				return err
			}
		}
		return RunValidation(dest)
	}

	ptr := val.UnsafePointer()

	// 2. Extract multi-source bindings
	for i := range desc.Bindings {
		b := &desc.Bindings[i]
		fieldPtr := unsafe.Add(ptr, b.Offset)

		switch b.Source {
		case SourceContext:
			if err := injectContextField(req, ptr, b); err != nil {
				return err
			}
			continue

		case SourceFile:
			file, err := req.FormFile(b.Key)
			if err != nil || file == nil {
				if b.Required {
					return fmt.Errorf("required file %q is missing", b.Key)
				}
				continue
			}
			reflect.NewAt(b.FieldType, fieldPtr).Elem().Set(reflect.ValueOf(file))
			continue

		case SourceFiles:
			files, err := req.FormFiles(b.Key)
			if err != nil || len(files) == 0 {
				if b.Required {
					return fmt.Errorf("required files %q are missing", b.Key)
				}
				continue
			}
			reflect.NewAt(b.FieldType, fieldPtr).Elem().Set(reflect.ValueOf(files))
			continue

		case SourceBodyRaw:
			data := req.Body()
			if len(data) == 0 && b.Required {
				return ErrEmptyRequestBody
			}
			*(*[]byte)(fieldPtr) = data
			continue

		case SourceBodyString:
			data := req.Body()
			if len(data) == 0 && b.Required {
				return ErrEmptyRequestBody
			}
			*(*string)(fieldPtr) = string(data)
			continue
		}

		var (
			rawVal  string
			present bool
		)

		switch b.Source {
		case SourcePath:
			rawVal = req.Param(b.Key)
			present = rawVal != ""
			if !present {
				if b.DefaultValue != "" {
					rawVal = b.DefaultValue
					present = true
				} else {
					return fmt.Errorf("missing path param %q", b.Key)
				}
			}

		case SourceQuery:
			rawVal = req.Query(b.Key)
			if b.IsSlice {
				present = rawVal != "" || len(req.RawURLQuery()[b.Key]) > 0
			} else {
				present = rawVal != ""
			}
			if !present {
				if b.DefaultValue != "" {
					rawVal = b.DefaultValue
					present = true
				} else if b.Required {
					return fmt.Errorf("missing query param %q", b.Key)
				}
			}

		case SourceHeader:
			rawVal = req.Header(b.Key)
			present = rawVal != ""
			if !present {
				if b.DefaultValue != "" {
					rawVal = b.DefaultValue
					present = true
				} else if b.Required {
					return fmt.Errorf("missing header %q", b.Key)
				}
			}

		case SourceCookie:
			c, err := req.Cookie(b.Key)
			present = err == nil && c != ""
			if present {
				rawVal = c
			} else {
				if b.DefaultValue != "" {
					rawVal = b.DefaultValue
					present = true
				} else if b.Required {
					return fmt.Errorf("missing cookie %q", b.Key)
				}
			}

		case SourceAuth:
			if strings.EqualFold(b.Key, "bearer") {
				token, ok := req.BearerToken()
				if ok {
					rawVal = token
					present = true
				} else if b.Required {
					return ErrMissingBearerToken
				}
			}

		case SourceNet:
			switch strings.ToLower(b.Key) {
			case "ip", "client_ip", "remote_ip":
				rawVal = req.ClientIP()
				present = rawVal != ""
			case "proto", "protocol":
				rawVal = req.Protocol()
				present = rawVal != ""
			case "scheme":
				rawVal = req.Scheme()
				present = rawVal != ""
			case "host":
				rawVal = req.Host()
				present = rawVal != ""
			case "method":
				rawVal = req.Method()
				present = rawVal != ""
			case "path":
				rawVal = req.Path()
				present = rawVal != ""
			}

		case SourceForm:
			rawVal = req.FormValue(b.Key)
			present = rawVal != ""
			if !present {
				if b.DefaultValue != "" {
					rawVal = b.DefaultValue
					present = true
				} else if b.Required {
					return fmt.Errorf("missing form field %q", b.Key)
				}
			}
		}

		if !present {
			continue
		}

		rawVal = ApplyTransforms(rawVal, b)

		if err := ValidateConstraints(rawVal, b); err != nil {
			return err
		}

		if b.IsSlice {
			if err := AssignSlice(req, fieldPtr, b, rawVal); err != nil {
				return err
			}
			continue
		}

		targetKind := b.Kind
		targetPtr := fieldPtr
		targetType := b.FieldType

		if b.IsPtr {
			targetKind = b.ElemKind
			targetType = b.FieldType.Elem()
			valPtr := reflect.New(targetType).UnsafePointer()
			*(*unsafe.Pointer)(fieldPtr) = valPtr
			targetPtr = valPtr
		}

		if err := AssignScalar(targetPtr, targetKind, targetType, rawVal, b.Source, b.Key); err != nil {
			return err
		}
	}

	// 3. Extract JSON body if fields are defined
	if desc.HasBodyFields && len(req.Body()) > 0 {
		if err := req.BindJSON(dest); err != nil {
			return err
		}
	}

	return RunValidation(dest)
}

func injectContextField(req RequestView, structPtr unsafe.Pointer, b *FieldBinding) error {
	val, ok := req.GetContext(b.FieldType)
	if ok {
		fieldPtr := unsafe.Add(structPtr, b.Offset)
		reflect.NewAt(b.FieldType, fieldPtr).Elem().Set(reflect.ValueOf(val))
		return nil
	}

	if b.Required {
		return fmt.Errorf("missing context value of type %s", b.FieldType.String())
	}
	return nil
}
