// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import "net/http"

// --- 4xx Client Error Domain Constructors ---

// BadRequest creates a 400 Bad Request domain error sentinel.
func BadRequest(code string, message ...string) DefinedError {
	return DefineError(http.StatusBadRequest, code, resolveMessage(code, message...))
}

// Unauthorized creates a 401 Unauthorized domain error sentinel.
func Unauthorized(code string, message ...string) DefinedError {
	return DefineError(http.StatusUnauthorized, code, resolveMessage(code, message...))
}

// PaymentRequired creates a 402 Payment Required domain error sentinel.
func PaymentRequired(code string, message ...string) DefinedError {
	return DefineError(http.StatusPaymentRequired, code, resolveMessage(code, message...))
}

// Forbidden creates a 403 Forbidden domain error sentinel.
func Forbidden(code string, message ...string) DefinedError {
	return DefineError(http.StatusForbidden, code, resolveMessage(code, message...))
}

// NotFound creates a 404 Not Found domain error sentinel.
func NotFound(code string, message ...string) DefinedError {
	return DefineError(http.StatusNotFound, code, resolveMessage(code, message...))
}

// MethodNotAllowed creates a 405 Method Not Allowed domain error sentinel.
func MethodNotAllowed(code string, message ...string) DefinedError {
	return DefineError(http.StatusMethodNotAllowed, code, resolveMessage(code, message...))
}

// NotAcceptable creates a 406 Not Acceptable domain error sentinel.
func NotAcceptable(code string, message ...string) DefinedError {
	return DefineError(http.StatusNotAcceptable, code, resolveMessage(code, message...))
}

// ProxyAuthRequired creates a 407 Proxy Authentication Required domain error sentinel.
func ProxyAuthRequired(code string, message ...string) DefinedError {
	return DefineError(http.StatusProxyAuthRequired, code, resolveMessage(code, message...))
}

// RequestTimeout creates a 408 Request Timeout domain error sentinel.
func RequestTimeout(code string, message ...string) DefinedError {
	return DefineError(http.StatusRequestTimeout, code, resolveMessage(code, message...))
}

// Conflict creates a 409 Conflict domain error sentinel.
func Conflict(code string, message ...string) DefinedError {
	return DefineError(http.StatusConflict, code, resolveMessage(code, message...))
}

// Gone creates a 410 Gone domain error sentinel.
func Gone(code string, message ...string) DefinedError {
	return DefineError(http.StatusGone, code, resolveMessage(code, message...))
}

// LengthRequired creates a 411 Length Required domain error sentinel.
func LengthRequired(code string, message ...string) DefinedError {
	return DefineError(http.StatusLengthRequired, code, resolveMessage(code, message...))
}

// PreconditionFailed creates a 412 Precondition Failed domain error sentinel.
func PreconditionFailed(code string, message ...string) DefinedError {
	return DefineError(http.StatusPreconditionFailed, code, resolveMessage(code, message...))
}

// PayloadTooLarge creates a 413 Payload Too Large domain error sentinel.
func PayloadTooLarge(code string, message ...string) DefinedError {
	return DefineError(http.StatusRequestEntityTooLarge, code, resolveMessage(code, message...))
}

// URITooLong creates a 414 URI Too Long domain error sentinel.
func URITooLong(code string, message ...string) DefinedError {
	return DefineError(http.StatusRequestURITooLong, code, resolveMessage(code, message...))
}

// UnsupportedMediaType creates a 415 Unsupported Media Type domain error sentinel.
func UnsupportedMediaType(code string, message ...string) DefinedError {
	return DefineError(http.StatusUnsupportedMediaType, code, resolveMessage(code, message...))
}

// RangeNotSatisfiable creates a 416 Range Not Satisfiable domain error sentinel.
func RangeNotSatisfiable(code string, message ...string) DefinedError {
	return DefineError(http.StatusRequestedRangeNotSatisfiable, code, resolveMessage(code, message...))
}

// ExpectationFailed creates a 417 Expectation Failed domain error sentinel.
func ExpectationFailed(code string, message ...string) DefinedError {
	return DefineError(http.StatusExpectationFailed, code, resolveMessage(code, message...))
}

// Teapot creates a 418 I'm a teapot domain error sentinel.
func Teapot(code string, message ...string) DefinedError {
	return DefineError(http.StatusTeapot, code, resolveMessage(code, message...))
}

// MisdirectedRequest creates a 421 Misdirected Request domain error sentinel.
func MisdirectedRequest(code string, message ...string) DefinedError {
	return DefineError(http.StatusMisdirectedRequest, code, resolveMessage(code, message...))
}

// Unprocessable creates a 422 Unprocessable Entity domain error sentinel.
func Unprocessable(code string, message ...string) DefinedError {
	return DefineError(http.StatusUnprocessableEntity, code, resolveMessage(code, message...))
}

// UnprocessableEntity is an alias for Unprocessable (422).
func UnprocessableEntity(code string, message ...string) DefinedError {
	return Unprocessable(code, message...)
}

// Locked creates a 423 Locked domain error sentinel.
func Locked(code string, message ...string) DefinedError {
	return DefineError(http.StatusLocked, code, resolveMessage(code, message...))
}

// FailedDependency creates a 424 Failed Dependency domain error sentinel.
func FailedDependency(code string, message ...string) DefinedError {
	return DefineError(http.StatusFailedDependency, code, resolveMessage(code, message...))
}

// TooEarly creates a 425 Too Early domain error sentinel.
func TooEarly(code string, message ...string) DefinedError {
	return DefineError(http.StatusTooEarly, code, resolveMessage(code, message...))
}

// UpgradeRequired creates a 426 Upgrade Required domain error sentinel.
func UpgradeRequired(code string, message ...string) DefinedError {
	return DefineError(http.StatusUpgradeRequired, code, resolveMessage(code, message...))
}

// PreconditionRequired creates a 428 Precondition Required domain error sentinel.
func PreconditionRequired(code string, message ...string) DefinedError {
	return DefineError(http.StatusPreconditionRequired, code, resolveMessage(code, message...))
}

// TooManyRequests creates a 429 Too Many Requests domain error sentinel.
func TooManyRequests(code string, message ...string) DefinedError {
	return DefineError(http.StatusTooManyRequests, code, resolveMessage(code, message...))
}

// HeaderFieldsTooLarge creates a 431 Request Header Fields Too Large domain error sentinel.
func HeaderFieldsTooLarge(code string, message ...string) DefinedError {
	return DefineError(http.StatusRequestHeaderFieldsTooLarge, code, resolveMessage(code, message...))
}

// UnavailableForLegalReasons creates a 451 Unavailable For Legal Reasons domain error sentinel.
func UnavailableForLegalReasons(code string, message ...string) DefinedError {
	return DefineError(http.StatusUnavailableForLegalReasons, code, resolveMessage(code, message...))
}

// --- 5xx Server Error Domain Constructors ---

// Internal creates a 500 Internal Server Error domain error sentinel.
func Internal(code string, message ...string) DefinedError {
	return DefineError(http.StatusInternalServerError, code, resolveMessage(code, message...))
}

// InternalServerError is an alias for Internal (500).
func InternalServerError(code string, message ...string) DefinedError {
	return Internal(code, message...)
}

// NotImplemented creates a 501 Not Implemented domain error sentinel.
func NotImplemented(code string, message ...string) DefinedError {
	return DefineError(http.StatusNotImplemented, code, resolveMessage(code, message...))
}

// BadGateway creates a 502 Bad Gateway domain error sentinel.
func BadGateway(code string, message ...string) DefinedError {
	return DefineError(http.StatusBadGateway, code, resolveMessage(code, message...))
}

// ServiceUnavailable creates a 503 Service Unavailable domain error sentinel.
func ServiceUnavailable(code string, message ...string) DefinedError {
	return DefineError(http.StatusServiceUnavailable, code, resolveMessage(code, message...))
}

// GatewayTimeout creates a 504 Gateway Timeout domain error sentinel.
func GatewayTimeout(code string, message ...string) DefinedError {
	return DefineError(http.StatusGatewayTimeout, code, resolveMessage(code, message...))
}

// HTTPVersionNotSupported creates a 505 HTTP Version Not Supported domain error sentinel.
func HTTPVersionNotSupported(code string, message ...string) DefinedError {
	return DefineError(http.StatusHTTPVersionNotSupported, code, resolveMessage(code, message...))
}

// VariantAlsoNegotiates creates a 506 Variant Also Negotiates domain error sentinel.
func VariantAlsoNegotiates(code string, message ...string) DefinedError {
	return DefineError(http.StatusVariantAlsoNegotiates, code, resolveMessage(code, message...))
}

// InsufficientStorage creates a 507 Insufficient Storage domain error sentinel.
func InsufficientStorage(code string, message ...string) DefinedError {
	return DefineError(http.StatusInsufficientStorage, code, resolveMessage(code, message...))
}

// LoopDetected creates a 508 Loop Detected domain error sentinel.
func LoopDetected(code string, message ...string) DefinedError {
	return DefineError(http.StatusLoopDetected, code, resolveMessage(code, message...))
}

// NotExtended creates a 510 Not Extended domain error sentinel.
func NotExtended(code string, message ...string) DefinedError {
	return DefineError(http.StatusNotExtended, code, resolveMessage(code, message...))
}

// NetworkAuthRequired creates a 511 Network Authentication Required domain error sentinel.
func NetworkAuthRequired(code string, message ...string) DefinedError {
	return DefineError(http.StatusNetworkAuthenticationRequired, code, resolveMessage(code, message...))
}
