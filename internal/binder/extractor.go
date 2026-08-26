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

// CompileExtractor builds the optimized extractor rule for a field at startup.
func CompileExtractor(src ParamSource, key string, required bool, defaultVal string, fieldType reflect.Type, isSlice bool) (ExtractorFunc, SpecialExtractorFunc) {
	switch src {
	case SourceContext:
		return nil, func(req RequestView, fieldPtr unsafe.Pointer) error {
			val, ok := req.GetContext(fieldType)
			if ok {
				reflect.NewAt(fieldType, fieldPtr).Elem().Set(reflect.ValueOf(val))
				return nil
			}
			if required {
				return fmt.Errorf("missing context value of type %s", fieldType.String())
			}
			return nil
		}

	case SourceFile:
		return nil, func(req RequestView, fieldPtr unsafe.Pointer) error {
			file, err := req.FormFile(key)
			if err != nil || file == nil {
				if required {
					return fmt.Errorf("required file %q is missing", key)
				}
				return nil
			}
			reflect.NewAt(fieldType, fieldPtr).Elem().Set(reflect.ValueOf(file))
			return nil
		}

	case SourceFiles:
		return nil, func(req RequestView, fieldPtr unsafe.Pointer) error {
			files, err := req.FormFiles(key)
			if err != nil || len(files) == 0 {
				if required {
					return fmt.Errorf("required files %q are missing", key)
				}
				return nil
			}
			reflect.NewAt(fieldType, fieldPtr).Elem().Set(reflect.ValueOf(files))
			return nil
		}

	case SourceBodyRaw:
		return nil, func(req RequestView, fieldPtr unsafe.Pointer) error {
			data := req.Body()
			if len(data) == 0 && required {
				return ErrEmptyRequestBody
			}
			*(*[]byte)(fieldPtr) = data
			return nil
		}

	case SourceBodyString:
		return nil, func(req RequestView, fieldPtr unsafe.Pointer) error {
			data := req.Body()
			if len(data) == 0 && required {
				return ErrEmptyRequestBody
			}
			*(*string)(fieldPtr) = string(data)
			return nil
		}

	case SourcePath:
		return func(req RequestView) (string, bool, error) {
			raw := req.Param(key)
			if raw == "" {
				if defaultVal != "" {
					return defaultVal, true, nil
				}
				return "", false, fmt.Errorf("missing path param %q", key)
			}
			return raw, true, nil
		}, nil

	case SourceQuery:
		return func(req RequestView) (string, bool, error) {
			raw := req.Query(key)
			present := raw != ""
			if isSlice && !present {
				present = len(req.RawURLQuery()[key]) > 0
			}
			if !present {
				if defaultVal != "" {
					return defaultVal, true, nil
				}
				if required {
					return "", false, fmt.Errorf("missing query param %q", key)
				}
				return "", false, nil
			}
			return raw, true, nil
		}, nil

	case SourceHeader:
		return func(req RequestView) (string, bool, error) {
			raw := req.Header(key)
			if raw == "" {
				if defaultVal != "" {
					return defaultVal, true, nil
				}
				if required {
					return "", false, fmt.Errorf("missing header %q", key)
				}
				return "", false, nil
			}
			return raw, true, nil
		}, nil

	case SourceCookie:
		return func(req RequestView) (string, bool, error) {
			c, err := req.Cookie(key)
			if err != nil || c == "" {
				if defaultVal != "" {
					return defaultVal, true, nil
				}
				if required {
					return "", false, fmt.Errorf("missing cookie %q", key)
				}
				return "", false, nil
			}
			return c, true, nil
		}, nil

	case SourceAuth:
		if strings.EqualFold(key, "bearer") {
			return func(req RequestView) (string, bool, error) {
				token, ok := req.BearerToken()
				if !ok || token == "" {
					if required {
						return "", false, ErrMissingBearerToken
					}
					return "", false, nil
				}
				return token, true, nil
			}, nil
		}

	case SourceNet:
		netKey := strings.ToLower(key)
		return func(req RequestView) (string, bool, error) {
			var raw string
			switch netKey {
			case "ip", "client_ip", "remote_ip":
				raw = req.ClientIP()
			case "proto", "protocol":
				raw = req.Protocol()
			case "scheme":
				raw = req.Scheme()
			case "host":
				raw = req.Host()
			case "method":
				raw = req.Method()
			case "path":
				raw = req.Path()
			}
			if raw == "" {
				return "", false, nil
			}
			return raw, true, nil
		}, nil

	case SourceForm:
		return func(req RequestView) (string, bool, error) {
			raw := req.FormValue(key)
			if raw == "" {
				if defaultVal != "" {
					return defaultVal, true, nil
				}
				if required {
					return "", false, fmt.Errorf("missing form field %q", key)
				}
				return "", false, nil
			}
			return raw, true, nil
		}, nil
	}

	return func(req RequestView) (string, bool, error) {
		return "", false, nil
	}, nil
}
