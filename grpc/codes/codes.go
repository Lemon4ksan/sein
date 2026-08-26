// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package codes defines the standard canonical status codes used by gRPC.
package codes

import (
	"fmt"
	"strconv"
)

// A Code is an unsigned 32-bit error code as defined in the gRPC specification.
type Code uint32

const (
	// OK is returned on success.
	OK Code = 0

	// Canceled indicates the operation was canceled (typically by the caller).
	Canceled Code = 1

	// Unknown error. An example of where this error may be returned is
	// if a Status value received from another address space belongs to
	// an error-space that is not known in this address space.
	Unknown Code = 2

	// InvalidArgument indicates client specified an invalid argument.
	InvalidArgument Code = 3

	// DeadlineExceeded means operation expired before completion.
	DeadlineExceeded Code = 4

	// NotFound means some requested entity was not found.
	NotFound Code = 5

	// AlreadyExists means an entity that we attempted to create already exists.
	AlreadyExists Code = 6

	// PermissionDenied indicates the caller does not have permission to
	// execute the specified operation.
	PermissionDenied Code = 7

	// ResourceExhausted indicates some resource has been exhausted, perhaps
	// a per-user quota, or perhaps the entire file system is out of space.
	ResourceExhausted Code = 8

	// FailedPrecondition indicates operation was rejected because the
	// system is not in a state required for the operation's execution.
	FailedPrecondition Code = 9

	// Aborted indicates the operation was aborted, typically due to a
	// concurrency issue like sequencer check failures, transaction aborts, etc.
	Aborted Code = 10

	// OutOfRange means operation was attempted past the valid range.
	OutOfRange Code = 11

	// Unimplemented indicates operation is not implemented or not
	// supported/enabled in this service.
	Unimplemented Code = 12

	// Internal errors. Means some invariants expected by underlying
	// system has been broken.
	Internal Code = 13

	// Unavailable indicates the service is currently unavailable.
	Unavailable Code = 14

	// DataLoss indicates unrecoverable data loss or corruption.
	DataLoss Code = 15

	// Unauthenticated indicates the request does not have valid
	// authentication credentials for the operation.
	Unauthenticated Code = 16
)

var strToCode = map[string]Code{
	"OK":                  OK,
	"CANCELLED":           Canceled,
	"UNKNOWN":             Unknown,
	"INVALID_ARGUMENT":    InvalidArgument,
	"DEADLINE_EXCEEDED":   DeadlineExceeded,
	"NOT_FOUND":           NotFound,
	"ALREADY_EXISTS":      AlreadyExists,
	"PERMISSION_DENIED":   PermissionDenied,
	"RESOURCE_EXHAUSTED":  ResourceExhausted,
	"FAILED_PRECONDITION": FailedPrecondition,
	"ABORTED":             Aborted,
	"OUT_OF_RANGE":        OutOfRange,
	"UNIMPLEMENTED":       Unimplemented,
	"INTERNAL":            Internal,
	"UNAVAILABLE":         Unavailable,
	"DATA_LOSS":           DataLoss,
	"UNAUTHENTICATED":     Unauthenticated,
}

var codeToString = [17]string{
	"OK",
	"Canceled",
	"Unknown",
	"InvalidArgument",
	"DeadlineExceeded",
	"NotFound",
	"AlreadyExists",
	"PermissionDenied",
	"ResourceExhausted",
	"FailedPrecondition",
	"Aborted",
	"OutOfRange",
	"Unimplemented",
	"Internal",
	"Unavailable",
	"DataLoss",
	"Unauthenticated",
}

// String returns the canonical string representation of the Code.
func (c Code) String() string {
	if int(c) < len(codeToString) {
		return codeToString[c]
	}

	return "Code(" + strconv.FormatInt(int64(c), 10) + ")"
}

// UnmarshalJSON unmarshals a string or integer into a Code.
func (c *Code) UnmarshalJSON(b []byte) error {
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
		if code, ok := strToCode[s]; ok {
			*c = code
			return nil
		}
		return fmt.Errorf("invalid code %q", s)
	}

	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid code %q: %w", s, err)
	}
	*c = Code(n)

	return nil
}
