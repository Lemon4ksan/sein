// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"errors"
	"reflect"
	"strconv"

	"github.com/lemon4ksan/foundation/generic"
)

// ParamConstraint defines supported primitive and scalar types for URL and Header parameters.
type ParamConstraint interface {
	~string | ~uint64 | ~uint32 | ~uint16 | ~uint8 | ~uint |
		~int64 | ~int32 | ~int16 | ~int8 | ~int | ~bool | ~float64 | ~float32
}

// ParamSlot represents a key-value path parameter pair without heap allocation.
type ParamSlot struct {
	Key   string
	Value string
}

// Params holds parsed URL path parameters with zero heap allocations for up to 8 parameters.
type Params struct {
	slots [8]ParamSlot
	count int
	extra []ParamSlot
}

// Get returns the value of the parameter with key, or empty string if not found.
func (p *Params) Get(key string) string {
	if p == nil {
		return ""
	}

	for i := 0; i < p.count; i++ {
		if p.slots[i].Key == key {
			return p.slots[i].Value
		}
	}

	for _, slot := range p.extra {
		if slot.Key == key {
			return slot.Value
		}
	}

	return ""
}

// Find returns the value of key and true if found.
func (p *Params) Find(key string) (string, bool) {
	if p == nil {
		return "", false
	}

	for i := 0; i < p.count; i++ {
		if p.slots[i].Key == key {
			return p.slots[i].Value, true
		}
	}

	for _, slot := range p.extra {
		if slot.Key == key {
			return slot.Value, true
		}
	}

	return "", false
}

// Set sets a key-value parameter pair.
func (p *Params) Set(key, value string) {
	for i := 0; i < p.count; i++ {
		if p.slots[i].Key == key {
			p.slots[i].Value = value
			return
		}
	}

	for i := range p.extra {
		if p.extra[i].Key == key {
			p.extra[i].Value = value
			return
		}
	}

	if p.count < len(p.slots) {
		p.slots[p.count] = ParamSlot{Key: key, Value: value}
		p.count++
		return
	}

	p.extra = append(p.extra, ParamSlot{Key: key, Value: value})
}

// Reset clears all parameters.
func (p *Params) Reset() {
	p.count = 0
	if len(p.extra) > 0 {
		p.extra = p.extra[:0]
	}
}

// Len returns the number of parameters.
func (p *Params) Len() int {
	if p == nil {
		return 0
	}

	return p.count + len(p.extra)
}

// Map returns a copy of parameters as a map[string]string for compatibility.
func (p *Params) Map() map[string]string {
	if p == nil {
		return nil
	}

	total := p.count + len(p.extra)
	if total == 0 {
		return nil
	}

	m := make(map[string]string, total)
	for i := 0; i < p.count; i++ {
		m[p.slots[i].Key] = p.slots[i].Value
	}

	for _, slot := range p.extra {
		m[slot.Key] = slot.Value
	}

	return m
}

// ParamValue represents a raw path parameter or query string value.
type ParamValue string

// String returns the raw string value.
func (p ParamValue) String() string {
	return string(p)
}

// IsEmpty reports whether the parameter is empty.
func (p ParamValue) IsEmpty() bool {
	return len(p) == 0
}

// Int parses the parameter into an integer.
func (p ParamValue) Int() (int, error) {
	return strconv.Atoi(string(p))
}

// AsInt returns the parsed integer or the fallback default if parsing fails.
func (p ParamValue) AsInt(fallback ...int) int {
	v, err := p.Int()
	if err != nil {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return 0
	}

	return v
}

// Uint64 parses the parameter into a uint64.
func (p ParamValue) Uint64() (uint64, error) {
	return strconv.ParseUint(string(p), 10, 64)
}

// AsUint64 returns the parsed uint64 or the fallback default if parsing fails.
func (p ParamValue) AsUint64(fallback ...uint64) uint64 {
	v, err := p.Uint64()
	if err != nil {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return 0
	}

	return v
}

// Int64 parses the parameter into an int64.
func (p ParamValue) Int64() (int64, error) {
	return strconv.ParseInt(string(p), 10, 64)
}

// AsInt64 returns the parsed int64 or the fallback default if parsing fails.
func (p ParamValue) AsInt64(fallback ...int64) int64 {
	v, err := p.Int64()
	if err != nil {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return 0
	}

	return v
}

// Bool parses the parameter into a boolean.
func (p ParamValue) Bool() (bool, error) {
	return strconv.ParseBool(string(p))
}

// AsBool returns the parsed bool or the fallback default if parsing fails.
func (p ParamValue) AsBool(fallback ...bool) bool {
	v, err := p.Bool()
	if err != nil {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return false
	}

	return v
}

// PathParamDef is a typed path parameter descriptor.
type PathParamDef[T ParamConstraint] struct {
	name string
}

// PathParam defines a typed path parameter descriptor (e.g. sein.PathParam[types.Snowflake]("id")).
func PathParam[T ParamConstraint](name string) PathParamDef[T] {
	return PathParamDef[T]{name: name}
}

// Name returns the parameter key name.
func (p PathParamDef[T]) Name() string {
	return p.name
}

// Get extracts and parses the path parameter from the request into type T.
func (p PathParamDef[T]) Get(req *Request) (T, error) {
	val := req.Param(p.name)
	if val.IsEmpty() {
		return generic.Zero[T](), ErrMissingPathParam.WithDetail("param", p.name)
	}

	return parseScalar[T](string(val))
}

// MustGet extracts the path parameter or panics if invalid/missing.
func (p PathParamDef[T]) MustGet(req *Request) T {
	v, err := p.Get(req)
	if err != nil {
		panic(err)
	}

	return v
}

// GetOr extracts the path parameter or returns fallback if not valid.
func (p PathParamDef[T]) GetOr(req *Request, fallback T) T {
	v, err := p.Get(req)
	if err != nil {
		return fallback
	}

	return v
}

// QueryParamDef is a typed query parameter descriptor.
type QueryParamDef[T ParamConstraint] struct {
	name string
}

// QueryParam defines a typed query parameter descriptor (e.g. sein.QueryParam[int]("page")).
func QueryParam[T ParamConstraint](name string) QueryParamDef[T] {
	return QueryParamDef[T]{name: name}
}

// Name returns the query parameter key name.
func (q QueryParamDef[T]) Name() string {
	return q.name
}

// Get extracts and parses the query parameter from the request.
func (q QueryParamDef[T]) Get(req *Request) (T, error) {
	val := req.Query(q.name)
	if val.IsEmpty() {
		return generic.Zero[T](), ErrMissingQueryParam.WithDetail("param", q.name)
	}

	return parseScalar[T](string(val))
}

// GetOr extracts the query parameter or returns fallback if not provided.
func (q QueryParamDef[T]) GetOr(req *Request, fallback T) T {
	val := req.Query(q.name)
	if val.IsEmpty() {
		return fallback
	}

	v, err := parseScalar[T](string(val))
	if err != nil {
		return fallback
	}

	return v
}

// HeaderParamDef is a typed header descriptor.
type HeaderParamDef[T ParamConstraint] struct {
	name string
}

// HeaderParam defines a typed header descriptor (e.g. sein.HeaderParam[string]("X-Token")).
func HeaderParam[T ParamConstraint](name string) HeaderParamDef[T] {
	return HeaderParamDef[T]{name: name}
}

// Name returns the header key name.
func (h HeaderParamDef[T]) Name() string {
	return h.name
}

// Get extracts and parses the header value from the request.
func (h HeaderParamDef[T]) Get(req *Request) (T, error) {
	val := req.Header(h.name)
	if val == "" {
		return generic.Zero[T](), ErrMissingHeader.WithDetail("header", h.name)
	}

	return parseScalar[T](val)
}

// GetOr extracts the header or returns fallback if empty.
func (h HeaderParamDef[T]) GetOr(req *Request, fallback T) T {
	val := req.Header(h.name)
	if val == "" {
		return fallback
	}

	v, err := parseScalar[T](val)
	if err != nil {
		return fallback
	}

	return v
}

func parseScalar[T ParamConstraint](s string) (T, error) {
	var zero T
	switch any(zero).(type) {
	case string:
		return any(s).(T), nil
	case uint64:
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return zero, ErrBadRequest("invalid uint64 parameter", err)
		}

		return any(v).(T), nil

	case uint32:
		v, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			return zero, ErrBadRequest("invalid uint32 parameter", err)
		}

		return any(uint32(v)).(T), nil

	case uint16:
		v, err := strconv.ParseUint(s, 10, 16)
		if err != nil {
			return zero, ErrBadRequest("invalid uint16 parameter", err)
		}

		return any(uint16(v)).(T), nil

	case uint8:
		v, err := strconv.ParseUint(s, 10, 8)
		if err != nil {
			return zero, ErrBadRequest("invalid uint8 parameter", err)
		}

		return any(uint8(v)).(T), nil

	case uint:
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return zero, ErrBadRequest("invalid uint parameter", err)
		}

		return any(uint(v)).(T), nil

	case int64:
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return zero, ErrBadRequest("invalid int64 parameter", err)
		}

		return any(v).(T), nil

	case int32:
		v, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return zero, ErrBadRequest("invalid int32 parameter", err)
		}

		return any(int32(v)).(T), nil

	case int16:
		v, err := strconv.ParseInt(s, 10, 16)
		if err != nil {
			return zero, ErrBadRequest("invalid int16 parameter", err)
		}

		return any(int16(v)).(T), nil

	case int8:
		v, err := strconv.ParseInt(s, 10, 8)
		if err != nil {
			return zero, ErrBadRequest("invalid int8 parameter", err)
		}

		return any(int8(v)).(T), nil

	case int:
		v, err := strconv.Atoi(s)
		if err != nil {
			return zero, ErrBadRequest("invalid integer parameter", err)
		}

		return any(v).(T), nil

	case bool:
		v, err := strconv.ParseBool(s)
		if err != nil {
			return zero, ErrBadRequest("invalid boolean parameter", err)
		}

		return any(v).(T), nil

	case float64:
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return zero, ErrBadRequest("invalid float64 parameter", err)
		}

		return any(v).(T), nil

	case float32:
		v, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return zero, ErrBadRequest("invalid float32 parameter", err)
		}

		return any(float32(v)).(T), nil

	default:
		return parseUnderlying[T](s)
	}
}

func parseUnderlying[T ParamConstraint](s string) (T, error) {
	var zero T

	typ := reflect.TypeFor[T]()
	switch typ.Kind() {
	case reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8, reflect.Uint:
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return zero, ErrBadRequest("invalid unsigned integer parameter", err)
		}

		res := reflect.New(typ).Elem()
		res.SetUint(v)

		return res.Interface().(T), nil

	case reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8, reflect.Int:
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return zero, ErrBadRequest("invalid integer parameter", err)
		}

		res := reflect.New(typ).Elem()
		res.SetInt(v)

		return res.Interface().(T), nil

	case reflect.String:
		res := reflect.New(typ).Elem()
		res.SetString(s)
		return res.Interface().(T), nil
	case reflect.Bool:
		v, err := strconv.ParseBool(s)
		if err != nil {
			return zero, ErrBadRequest("invalid boolean parameter", err)
		}

		res := reflect.New(typ).Elem()
		res.SetBool(v)

		return res.Interface().(T), nil

	default:
		return zero, errors.New("sein: unsupported parameter scalar type")
	}
}
