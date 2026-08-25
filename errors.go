// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"errors"
	"fmt"
	"net/http"
)

// HTTPError is a semantic error with an associated HTTP status code and machine-readable message.
type HTTPError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Cause   error  `json:"-"`
}

func (e HTTPError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("HTTP %d: %s (cause: %v)", e.Status, e.Message, e.Cause)
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
}

func (e HTTPError) StatusCode() int {
	if e.Status == 0 {
		return http.StatusInternalServerError
	}
	return e.Status
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
		Message: message,
		Cause:   c,
	}
}

// ErrBadRequest creates a 400 Bad Request error.
func ErrBadRequest(message string, cause ...error) HTTPError {
	return NewError(http.StatusBadRequest, message, cause...)
}

// ErrUnauthorized creates a 401 Unauthorized error.
func ErrUnauthorized(message string, cause ...error) HTTPError {
	return NewError(http.StatusUnauthorized, message, cause...)
}

// ErrForbidden creates a 403 Forbidden error.
func ErrForbidden(message string, cause ...error) HTTPError {
	return NewError(http.StatusForbidden, message, cause...)
}

// ErrNotFound creates a 404 Not Found error.
func ErrNotFound(message string, cause ...error) HTTPError {
	return NewError(http.StatusNotFound, message, cause...)
}

// ErrConflict creates a 409 Conflict error.
func ErrConflict(message string, cause ...error) HTTPError {
	return NewError(http.StatusConflict, message, cause...)
}

// ErrUnprocessable creates a 422 Unprocessable Entity error.
func ErrUnprocessable(message string, cause ...error) HTTPError {
	return NewError(http.StatusUnprocessableEntity, message, cause...)
}

// ErrInternal creates a 500 Internal Server Error.
func ErrInternal(message string, cause ...error) HTTPError {
	return NewError(http.StatusInternalServerError, message, cause...)
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
