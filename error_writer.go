// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"errors"
	"net/http"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/net/http/header"

	"github.com/lemon4ksan/sein/internal/fast/h1engine"
	"github.com/lemon4ksan/sein/internal/fast/h2engine"
	"github.com/lemon4ksan/sein/internal/fast/h3engine"
)

type errorResponse struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func buildErrorResponse(err error, mappers []ErrorMapper) errorResponse {
	for _, mapper := range mappers {
		if mapped, ok := mapper(err); ok {
			err = mapped
			break
		}
	}

	var (
		definedErr DefinedError
		domainErr  DomainError
		httpErr    HTTPError
	)

	switch {
	case errors.As(err, &definedErr):
		return errorResponse{
			Status:  definedErr.HTTPStatus(),
			Code:    definedErr.ErrorCode(),
			Message: definedErr.Message(),
			Details: definedErr.Details(),
		}

	case errors.As(err, &domainErr):
		return errorResponse{
			Status:  domainErr.HTTPStatus(),
			Code:    domainErr.ErrorCode(),
			Message: domainErr.Error(),
		}

	case errors.As(err, &httpErr):
		return errorResponse{
			Status:  httpErr.HTTPStatus(),
			Code:    httpErr.ErrorCode(),
			Message: httpErr.Message,
			Details: httpErr.Details,
		}

	default:
		return errorResponse{
			Status:  http.StatusInternalServerError,
			Code:    "INTERNAL_SERVER_ERROR",
			Message: err.Error(),
		}
	}
}

func (s *Server) writeH1Error(res *h1engine.Response, err error) {
	if redir, ok := errors.AsType[RedirectError](err); ok {
		res.StatusCode = redir.Status
		res.Headers.Set(header.Location, redir.TargetURL)
		res.Body = nil
		return
	}

	resp := buildErrorResponse(err, s.errorMappers)

	res.StatusCode = resp.Status
	res.Headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)

	data, _ := json.Marshal(resp)
	res.Body = data
}

func (s *Server) writeH2Error(res *h2engine.ServerResponse, err error) {
	if res.Headers == nil {
		res.Headers = make(http.Header)
	}

	if redir, ok := errors.AsType[RedirectError](err); ok {
		res.StatusCode = redir.Status
		res.Headers.Set(header.Location, redir.TargetURL)
		res.Body = nil
		return
	}

	resp := buildErrorResponse(err, s.errorMappers)

	res.StatusCode = resp.Status
	res.Headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)

	data, _ := json.Marshal(resp)
	res.Body = data
}

func (s *Server) writeH3Error(res *h3engine.ServerResponse, err error) {
	if res.Headers == nil {
		res.Headers = make(http.Header)
	}

	if redir, ok := errors.AsType[RedirectError](err); ok {
		res.StatusCode = redir.Status
		res.Headers.Set(header.Location, redir.TargetURL)
		res.Body = nil
		return
	}

	resp := buildErrorResponse(err, s.errorMappers)

	res.StatusCode = resp.Status
	res.Headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)

	data, _ := json.Marshal(resp)
	res.Body = data
}

func (s *Server) writeError(w http.ResponseWriter, err error) {
	if redir, ok := errors.AsType[RedirectError](err); ok {
		w.Header().Set(header.Location, redir.TargetURL)
		w.WriteHeader(redir.Status)
		return
	}

	resp := buildErrorResponse(err, s.errorMappers)

	w.Header().Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)
	w.WriteHeader(resp.Status)
	_ = json.NewEncoder(w).Encode(resp)
}
