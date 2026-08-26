// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"errors"
	"fmt"
	"net/http"
)

// DomainError is the standard interface for typed business domain errors.
// Any error implementing this interface automatically dictates its HTTP status code and machine-readable error code.
type DomainError interface {
	error
	HTTPStatus() int
	ErrorCode() string
}

// DefinedError is an immutable, zero-allocation domain error sentinel.
type DefinedError struct {
	status  int
	code    string
	message string
	details map[string]any
	cause   error
}

// DefineError creates a reusable, machine-readable domain error sentinel with a custom status code.
func DefineError(status int, code, message string) DefinedError {
	return DefinedError{
		status:  status,
		code:    code,
		message: message,
	}
}

func resolveMessage(code string, message ...string) string {
	if len(message) > 0 && message[0] != "" {
		return message[0]
	}

	return code
}

// Core framework sentinels (exported, customizable, checkable via errors.Is)
var (
	ErrMissingBearerToken = Unauthorized("MISSING_BEARER_TOKEN", "Authorization Bearer token is required")
	ErrInvalidBearerToken = Unauthorized("INVALID_BEARER_TOKEN", "Provided Bearer token is invalid or expired")
	ErrEmptyRequestBody   = BadRequest("EMPTY_REQUEST_BODY", "Request body cannot be empty")
	ErrInvalidJSONPayload = BadRequest("INVALID_JSON_PAYLOAD", "Invalid JSON payload structure")
	ErrValidationFailed   = BadRequest("VALIDATION_FAILED", "Request validation failed")
	ErrRouteNotFound      = NotFound("ROUTE_NOT_FOUND", "Requested route was not found")
	ErrInternalPanic      = Internal("INTERNAL_SERVER_PANIC", "An unexpected panic occurred")
	ErrMissingPathParam   = BadRequest("MISSING_PATH_PARAM", "Required path parameter is missing")
	ErrInvalidPathParam   = BadRequest("INVALID_PATH_PARAM", "Path parameter value is invalid")
	ErrMissingQueryParam  = BadRequest("MISSING_QUERY_PARAM", "Required query parameter is missing")
	ErrInvalidQueryParam  = BadRequest("INVALID_QUERY_PARAM", "Query parameter value is invalid")
	ErrMissingHeader      = BadRequest("MISSING_HEADER", "Required header is missing")
	ErrInvalidHeader      = BadRequest("INVALID_HEADER", "Header value is invalid")
	ErrMissingCookie      = BadRequest("MISSING_COOKIE", "Required cookie is missing")
	ErrInvalidCookie      = BadRequest("INVALID_COOKIE", "Cookie value is invalid")
	ErrMissingContext     = Unauthorized("MISSING_CONTEXT", "Required context value is missing")
)

func (d DefinedError) Error() string {
	if d.cause != nil {
		return fmt.Sprintf("[%s] %s (cause: %v)", d.code, d.message, d.cause)
	}

	return fmt.Sprintf("[%s] %s", d.code, d.message)
}

func (d DefinedError) HTTPStatus() int {
	if d.status == 0 {
		return http.StatusInternalServerError
	}

	return d.status
}

func (d DefinedError) ErrorCode() string {
	return d.code
}

func (d DefinedError) Message() string {
	return d.message
}

func (d DefinedError) Details() map[string]any {
	return d.details
}

func (d DefinedError) Unwrap() error {
	return d.cause
}

// WithCause wraps an underlying root-cause error.
func (d DefinedError) WithCause(err error) DefinedError {
	d.cause = err
	return d
}

// WithDetail adds a key-value detail field to the error payload.
func (d DefinedError) WithDetail(key string, val any) DefinedError {
	if d.details == nil {
		d.details = make(map[string]any)
	}

	d.details[key] = val

	return d
}

// WithMessage overrides the human-readable error message.
func (d DefinedError) WithMessage(msg string) DefinedError {
	d.message = msg
	return d
}

// HTTPError is a generic semantic error structure for ad-hoc runtime errors.
type HTTPError struct {
	Status  int            `json:"status"`
	Code    string         `json:"code,omitempty"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	Cause   error          `json:"-"`
}

func (e HTTPError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("HTTP %d [%s]: %s (cause: %v)", e.Status, e.Code, e.Message, e.Cause)
	}

	return fmt.Sprintf("HTTP %d [%s]: %s", e.Status, e.Code, e.Message)
}

func (e HTTPError) HTTPStatus() int {
	if e.Status == 0 {
		return http.StatusInternalServerError
	}

	return e.Status
}

func (e HTTPError) StatusCode() int {
	return e.HTTPStatus()
}

func (e HTTPError) ErrorCode() string {
	return e.Code
}

func (e HTTPError) Unwrap() error {
	return e.Cause
}

// NewError creates a custom semantic HTTPError.
func NewError(status int, message string, cause ...error) HTTPError {
	var c error
	if len(cause) > 0 {
		c = cause[0]
	}

	return HTTPError{
		Status:  status,
		Code:    http.StatusText(status),
		Message: message,
		Cause:   c,
	}
}

// ErrBadRequest creates a 400 Bad Request ad-hoc error.
func ErrBadRequest(message string, cause ...error) HTTPError {
	return NewError(http.StatusBadRequest, message, cause...)
}

// ErrUnauthorized creates a 401 Unauthorized ad-hoc error.
func ErrUnauthorized(message string, cause ...error) HTTPError {
	return NewError(http.StatusUnauthorized, message, cause...)
}

// ErrForbidden creates a 403 Forbidden ad-hoc error.
func ErrForbidden(message string, cause ...error) HTTPError {
	return NewError(http.StatusForbidden, message, cause...)
}

// ErrNotFound creates a 404 Not Found ad-hoc error.
func ErrNotFound(message string, cause ...error) HTTPError {
	return NewError(http.StatusNotFound, message, cause...)
}

// ErrConflict creates a 409 Conflict ad-hoc error.
func ErrConflict(message string, cause ...error) HTTPError {
	return NewError(http.StatusConflict, message, cause...)
}

// ErrUnprocessable creates a 422 Unprocessable Entity ad-hoc error.
func ErrUnprocessable(message string, cause ...error) HTTPError {
	return NewError(http.StatusUnprocessableEntity, message, cause...)
}

// ErrTooManyRequests creates a 429 Too Many Requests ad-hoc error.
func ErrTooManyRequests(message string, cause ...error) HTTPError {
	return NewError(http.StatusTooManyRequests, message, cause...)
}

// ErrInternal creates a 500 Internal Server Error ad-hoc error.
func ErrInternal(message string, cause ...error) HTTPError {
	return NewError(http.StatusInternalServerError, message, cause...)
}

// NewHTTPError creates a structured HTTPError with status, code, and message.
func NewHTTPError(status int, code, message string) HTTPError {
	return HTTPError{
		Status:  status,
		Code:    code,
		Message: message,
	}
}

// AsHTTPError checks if an error wraps or is an HTTPError.
func AsHTTPError(err error) (HTTPError, bool) {
	if err == nil {
		return HTTPError{}, false
	}

	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		return httpErr, true
	}

	return HTTPError{}, false
}
