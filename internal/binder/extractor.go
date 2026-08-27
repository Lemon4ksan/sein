// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package binder

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"github.com/lemon4ksan/foundation/generic"
)

// StringExtractorFunc extracts a raw string representation from a request wrapped in an Optional.
type StringExtractorFunc func(req RequestView) (generic.Optional[string], error)

// compileSpecialStep checks if the field source is non-string (Context, Files, Raw Body) and returns a FieldStep.
func compileSpecialStep(b *FieldBinding, offset uintptr) FieldStep {
	required := b.Required
	key := b.Key
	fieldType := b.FieldType

	switch b.Source {
	case SourceContext:
		return func(req RequestView, structPtr unsafe.Pointer) error {
			fieldPtr := unsafe.Add(structPtr, offset)

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
		return func(req RequestView, structPtr unsafe.Pointer) error {
			fieldPtr := unsafe.Add(structPtr, offset)

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
		return func(req RequestView, structPtr unsafe.Pointer) error {
			fieldPtr := unsafe.Add(structPtr, offset)

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
		return func(req RequestView, structPtr unsafe.Pointer) error {
			fieldPtr := unsafe.Add(structPtr, offset)

			data := req.Body()
			if len(data) == 0 && required {
				return ErrEmptyRequestBody
			}

			*(*[]byte)(fieldPtr) = data

			return nil
		}

	case SourceBodyString:
		return func(req RequestView, structPtr unsafe.Pointer) error {
			fieldPtr := unsafe.Add(structPtr, offset)

			data := req.Body()
			if len(data) == 0 && required {
				return ErrEmptyRequestBody
			}

			*(*string)(fieldPtr) = string(data)

			return nil
		}
	}

	return nil
}

// compileStringExtractor creates a StringExtractorFunc for standard sources returning generic.Optional.
func compileStringExtractor(b *FieldBinding) StringExtractorFunc {
	key := b.Key
	defaultVal := b.DefaultValue
	required := b.Required
	isSlice := b.IsSlice

	switch b.Source {
	case SourcePath:
		return func(req RequestView) (generic.Optional[string], error) {
			raw := generic.Coalesce(req.Param(key), defaultVal)
			if raw == "" {
				return generic.None[string](), fmt.Errorf("missing path param %q", key)
			}

			return generic.Some(raw), nil
		}

	case SourceQuery:
		return func(req RequestView) (generic.Optional[string], error) {
			raw := req.Query(key)

			present := raw != ""
			if isSlice && !present {
				present = len(req.RawURLQuery()[key]) > 0
			}

			if !present {
				if defaultVal != "" {
					return generic.Some(defaultVal), nil
				}

				if required {
					return generic.None[string](), fmt.Errorf("missing query param %q", key)
				}

				return generic.None[string](), nil
			}

			return generic.Some(raw), nil
		}

	case SourceHeader:
		return func(req RequestView) (generic.Optional[string], error) {
			raw := generic.Coalesce(req.Header(key), defaultVal)
			if raw == "" {
				if required {
					return generic.None[string](), fmt.Errorf("missing header %q", key)
				}

				return generic.None[string](), nil
			}

			return generic.Some(raw), nil
		}

	case SourceCookie:
		signed := b.Signed
		return func(req RequestView) (generic.Optional[string], error) {
			c, err := req.Cookie(key)

			raw := generic.Coalesce(c, defaultVal)
			if err != nil && raw == "" {
				if required {
					return generic.None[string](), fmt.Errorf("missing cookie %q", key)
				}

				return generic.None[string](), nil
			}

			if raw == "" {
				if required {
					return generic.None[string](), fmt.Errorf("missing cookie %q", key)
				}

				return generic.None[string](), nil
			}

			if signed {
				verified, ok := verifySignedCookie(raw, req.CookieSecret())
				if !ok {
					return generic.None[string](), ErrInvalidCookieSignature
				}
				raw = verified
			}

			return generic.Some(raw), nil
		}

	case SourceAuth:
		if strings.EqualFold(key, "bearer") {
			return func(req RequestView) (generic.Optional[string], error) {
				token, ok := req.BearerToken()
				if !ok || token == "" {
					if required {
						return generic.None[string](), ErrMissingBearerToken
					}

					return generic.None[string](), nil
				}

				return generic.Some(token), nil
			}
		}

	case SourceNet:
		netKey := strings.ToLower(key)

		return func(req RequestView) (generic.Optional[string], error) {
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
				return generic.None[string](), nil
			}

			return generic.Some(raw), nil
		}

	case SourceForm:
		return func(req RequestView) (generic.Optional[string], error) {
			raw := generic.Coalesce(req.FormValue(key), defaultVal)
			if raw == "" {
				if required {
					return generic.None[string](), fmt.Errorf("missing form field %q", key)
				}

				return generic.None[string](), nil
			}

			return generic.Some(raw), nil
		}
	}

	return func(req RequestView) (generic.Optional[string], error) {
		return generic.None[string](), nil
	}
}

func verifySignedCookie(signedValue, secret string) (string, bool) {
	if secret == "" {
		return signedValue, true
	}
	idx := strings.LastIndexByte(signedValue, '.')
	if idx < 0 {
		return "", false
	}
	value := signedValue[:idx]
	sigHex := signedValue[idx+1:]

	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return "", false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(sig, expectedSig) {
		return "", false
	}

	return value, true
}
