// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/lemon4ksan/foundation/codec/json"
)

// HTTPDoer abstracts any HTTP client (such as *http.Client or *aoni.Client).
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Call executes a strongly-typed HTTP RPC request, encoding the request body as JSON and decoding the response JSON into *Res.
// Path parameters in the format :param are automatically populated from matching struct fields with path tags.
func Call[Res, Req any](
	ctx context.Context,
	client HTTPDoer,
	method string,
	urlPattern string,
	reqPayload Req,
) (*Res, error) {
	resolvedURL := resolvePathParams(urlPattern, reqPayload)

	var bodyReader io.Reader
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodDelete {
		data, err := json.Marshal(reqPayload)
		if err != nil {
			return nil, fmt.Errorf("rpc: failed to encode request payload: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, resolvedURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("rpc: failed to construct request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	httpRes, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("rpc: network execution failed: %w", err)
	}
	defer func() {
		_ = httpRes.Body.Close()
	}()

	if httpRes.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(httpRes.Body)
		return nil, fmt.Errorf("rpc: server returned status %d: %s", httpRes.StatusCode, string(bodyBytes))
	}

	var result Res
	if err := json.NewDecoder(httpRes.Body).Decode(&result); err != nil {
		if err == io.EOF {
			return &result, nil
		}
		return nil, fmt.Errorf("rpc: failed to decode response JSON: %w", err)
	}

	return &result, nil
}

func resolvePathParams(pattern string, payload any) string {
	val := reflect.ValueOf(payload)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return pattern
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return pattern
	}

	typ := val.Type()
	res := pattern
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		pathTag := field.Tag.Get("path")
		if pathTag == "" {
			continue
		}

		fieldVal := fmt.Sprintf("%v", val.Field(i).Interface())
		res = strings.ReplaceAll(res, ":"+pathTag, fieldVal)
	}

	return res
}
