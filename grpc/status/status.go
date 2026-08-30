// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package status implements gRPC status errors and conversions.
package status

import (
	"errors"
	"fmt"

	"github.com/lemon4ksan/sein/grpc/codes"
)

// Status represents an RPC status code and message.
type Status struct {
	code    codes.Code
	message string
}

// New returns a Status representing c and msg.
func New(c codes.Code, msg string) *Status {
	return &Status{code: c, message: msg}
}

// Newf returns New(c, fmt.Sprintf(format, a...)).
func Newf(c codes.Code, format string, a ...any) *Status {
	return New(c, fmt.Sprintf(format, a...))
}

// Error returns an error representing c and msg. If c is OK, returns nil.
func Error(c codes.Code, msg string) error {
	return New(c, msg).Err()
}

// Errorf returns Error(c, fmt.Sprintf(format, a...)).
func Errorf(c codes.Code, format string, a ...any) error {
	return Error(c, fmt.Sprintf(format, a...))
}

// Code returns the status code.
func (s *Status) Code() codes.Code {
	if s == nil {
		return codes.OK
	}

	return s.code
}

// Message returns the status message.
func (s *Status) Message() string {
	if s == nil {
		return ""
	}

	return s.message
}

// Err returns an error containing the status code and message, or nil if Code is OK.
func (s *Status) Err() error {
	if s == nil || s.code == codes.OK {
		return nil
	}

	return &statusError{s: s}
}

// String returns the string representation of Status.
func (s *Status) String() string {
	return fmt.Sprintf("rpc error: code = %s desc = %s", s.Code(), s.Message())
}

type statusError struct {
	s *Status
}

func (e *statusError) Error() string {
	return e.s.String()
}

// GRPCStatus returns the Status representation of this error.
func (e *statusError) GRPCStatus() *Status {
	return e.s
}

// Is reports whether target matches this error.
func (e *statusError) Is(target error) bool {
	if se, ok := errors.AsType[*statusError](target); ok {
		return se.s.code == e.s.code && se.s.message == e.s.message
	}

	return false
}

// FromError returns a Status representation of err.
//
// If err was created by status.Error, it returns the Status and true.
// If err is nil, it returns a Status with code OK and true.
// Otherwise, it returns a Status with code Unknown and false.
func FromError(err error) (*Status, bool) {
	if err == nil {
		return New(codes.OK, ""), true
	}

	var se interface{ GRPCStatus() *Status }
	if errors.As(err, &se) {
		return se.GRPCStatus(), true
	}

	return New(codes.Unknown, err.Error()), false
}

// Convert is a convenience function that turns any error into a Status.
func Convert(err error) *Status {
	s, _ := FromError(err)

	return s
}

// Code returns the Code of the error if it is a Status error, or Unknown if not.
// Returns OK if err is nil.
func Code(err error) codes.Code {
	if err == nil {
		return codes.OK
	}

	return Convert(err).Code()
}
