// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sein

import (
	"net/http"

	"github.com/lemon4ksan/foundation/codec/json"
	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/net/http/header"
	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/sein/internal/fast/h1engine"
	"github.com/lemon4ksan/sein/internal/fast/h2engine"
	"github.com/lemon4ksan/sein/internal/fast/h3engine"
)

// copyHTTPHeaders copies all key-value pairs from src into dst.
func copyHTTPHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// serializePayload resolves any return value into an HTTP status code, headers, and body bytes.
func serializePayload(result any) (statusCode int, headers http.Header, body []byte, cookies []*http.Cookie, err error) {
	statusCode = http.StatusOK

	if holder, ok := result.(ResponseHolder); ok {
		statusCode = generic.Coalesce(holder.StatusCode(), http.StatusOK)
		headers = holder.ResponseHeaders()
		if headers == nil {
			headers = make(http.Header)
		}

		cookies = holder.ResponseCookies()

		rawBody := holder.ResponseBody()
		switch b := rawBody.(type) {
		case nil:
			return statusCode, headers, nil, cookies, nil
		case []byte:
			return statusCode, headers, b, cookies, nil
		case string:
			return statusCode, headers, bytesconv.S2B(b), cookies, nil
		default:
			data, err := json.Marshal(b)
			if err != nil {
				return statusCode, headers, nil, cookies, err
			}

			if headers.Get(header.ContentType) == "" {
				headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)
			}

			return statusCode, headers, data, cookies, nil
		}
	}

	headers = make(http.Header)

	switch v := result.(type) {
	case nil:
		return statusCode, headers, nil, nil, nil
	case []byte:
		headers.Set(header.ContentType, header.MIMEApplicationOctetStream)
		return statusCode, headers, v, nil, nil
	case string:
		headers.Set(header.ContentType, header.MIMETextPlainCharsetUTF8)
		return statusCode, headers, bytesconv.S2B(v), nil, nil
	default:
		headers.Set(header.ContentType, header.MIMEApplicationJSONCharsetUTF8)
		data, err := json.Marshal(v)
		if err != nil {
			return statusCode, headers, nil, nil, err
		}
		return statusCode, headers, data, nil, nil
	}
}

func (s *Server) serializeH1Result(res *h1engine.Response, result any) error {
	status, headers, body, cookies, err := serializePayload(result)
	if err != nil {
		return err
	}

	res.StatusCode = status
	res.Headers.AddFromHTTP(headers)
	res.Cookies = append(res.Cookies, cookies...)
	res.Body = body

	return nil
}

func (s *Server) serializeH2Result(res *h2engine.ServerResponse, result any) error {
	status, headers, body, _, err := serializePayload(result)
	if err != nil {
		return err
	}

	res.StatusCode = status
	if res.Headers == nil {
		res.Headers = make(http.Header, len(headers))
	}
	copyHTTPHeaders(res.Headers, headers)
	res.Body = body

	return nil
}

func (s *Server) serializeH3Result(res *h3engine.ServerResponse, result any) error {
	status, headers, body, _, err := serializePayload(result)
	if err != nil {
		return err
	}

	res.StatusCode = status
	if res.Headers == nil {
		res.Headers = make(http.Header, len(headers))
	}
	copyHTTPHeaders(res.Headers, headers)
	res.Body = body

	return nil
}
