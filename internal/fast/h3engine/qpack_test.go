// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3engine_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/sein/internal/fast/h3engine"
	"github.com/lemon4ksan/sein/internal/qpack"
)

func TestQPACK_ServerEncodeDecodeRoundtrip(t *testing.T) {
	codec := h3engine.NewQPACKCodec()

	// 1. Encode client request headers into QPACK block
	var clientBuf bytes.Buffer

	enc := qpack.NewEncoder(&clientBuf)
	require.NoError(t, enc.WriteField(qpack.HeaderField{Name: ":method", Value: "POST"}))
	require.NoError(t, enc.WriteField(qpack.HeaderField{Name: ":path", Value: "/api/v1/users"}))
	require.NoError(t, enc.WriteField(qpack.HeaderField{Name: ":scheme", Value: "https"}))
	require.NoError(t, enc.WriteField(qpack.HeaderField{Name: ":authority", Value: "api.example.com"}))
	require.NoError(t, enc.WriteField(qpack.HeaderField{Name: "user-agent", Value: "sein-h3-client"}))
	require.NoError(t, enc.WriteField(qpack.HeaderField{Name: "content-type", Value: "application/json"}))

	// 2. Server decodes request headers
	method, path, scheme, authority, headers, err := codec.DecodeRequestHeaders(clientBuf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, "POST", method)
	assert.Equal(t, "/api/v1/users", path)
	assert.Equal(t, "https", scheme)
	assert.Equal(t, "api.example.com", authority)
	assert.Equal(t, "sein-h3-client", headers.Get("user-agent"))
	assert.Equal(t, "application/json", headers.Get("content-type"))

	// 3. Server encodes response headers
	respHeaders := make(http.Header)
	respHeaders.Set("Server", "Sein/2.0")
	respHeaders.Set("Content-Type", "application/json")
	respHeaders.Set("X-Custom-Header", "Sein-Ultra-Fast")

	respBlock := codec.EncodeResponseHeaders(http.StatusOK, respHeaders, 42)

	// 4. Decode server response headers
	dec := qpack.NewDecoder()

	var (
		decodedStatus string
		decodedLen    string
		decodedServer string
		decodedCustom string
	)

	err = dec.DecodeFields(respBlock, func(hf qpack.HeaderField) bool {
		switch hf.Name {
		case ":status":
			decodedStatus = hf.Value
		case "content-length":
			decodedLen = hf.Value
		case "server":
			decodedServer = hf.Value
		case "x-custom-header":
			decodedCustom = hf.Value
		}

		return true
	})
	require.NoError(t, err)
	assert.Equal(t, "200", decodedStatus)
	assert.Equal(t, "42", decodedLen)
	assert.Equal(t, "Sein/2.0", decodedServer)
	assert.Equal(t, "Sein-Ultra-Fast", decodedCustom)
}
