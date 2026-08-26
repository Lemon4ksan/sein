// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package h3engine

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/lemon4ksan/foundation/silicon/bytesconv"

	"github.com/lemon4ksan/sein/internal/qpack"
)

var (
	ErrMissingMethodOrPath = errors.New("h3: missing :method or :path pseudo-header (RFC 9114 §4.1.2)")
	ErrMalformedHeader     = errors.New("h3: malformed header field (RFC 9114 §4.1.2)")
)

// PooledEncoder encapsulates a pooled buffer and QPACK encoder for zero-allocation serialization.
type PooledEncoder struct {
	buf *bytes.Buffer
	enc *qpack.Encoder
}

var encoderPool = sync.Pool{
	New: func() any {
		buf := new(bytes.Buffer)

		return &PooledEncoder{
			buf: buf,
			enc: qpack.NewEncoder(buf),
		}
	},
}

// QPACKCodec manages zero-allocation QPACK header serialization and deserialization (RFC 9204).
type QPACKCodec struct {
	decoder *qpack.Decoder
}

// NewQPACKCodec instantiates a new QPACKCodec.
func NewQPACKCodec() *QPACKCodec {
	return &QPACKCodec{
		decoder: qpack.NewDecoder(),
	}
}

// DecodeRequestHeaders decodes a QPACK header block into HTTP/3 pseudo-headers and standard http.Header.
func (q *QPACKCodec) DecodeRequestHeaders(
	headerBlock []byte,
) (method, path, scheme, authority string, headers http.Header, err error) {
	headers = make(http.Header)

	var (
		hasSeenRegularHeader bool
		malformed            bool
	)

	decodeErr := q.decoder.DecodeFields(headerBlock, func(hf qpack.HeaderField) bool {
		k := hf.Name
		v := hf.Value

		// RFC 9114 §4.1.2: All field names MUST be lowercase ASCII
		for i := 0; i < len(k); i++ {
			if k[i] >= 'A' && k[i] <= 'Z' {
				malformed = true
				return false
			}
		}

		if hf.IsPseudo() {
			// RFC 9114 §4.3: Pseudo-headers MUST appear before regular headers
			if hasSeenRegularHeader {
				malformed = true
				return false
			}

			switch k {
			case ":method":
				if method != "" {
					malformed = true
					return false
				}

				method = v

			case ":path":
				if path != "" {
					malformed = true
					return false
				}

				path = v

			case ":scheme":
				if scheme != "" {
					malformed = true
					return false
				}

				scheme = v

			case ":authority":
				if authority != "" {
					malformed = true
					return false
				}

				authority = v

			case ":protocol":
				// RFC 9220: Extended CONNECT for WebSockets
				headers.Set(":protocol", v)
			default:
				malformed = true
				return false
			}
		} else {
			hasSeenRegularHeader = true

			// RFC 9114 §4.1.2 & §4.1: Prohibited hop-by-hop headers in HTTP/3
			switch k {
			case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade":
				malformed = true
				return false
			case "te":
				if v != "trailers" {
					malformed = true
					return false
				}
			}

			headers.Add(k, v)
		}

		return true
	})
	if decodeErr != nil {
		return "", "", "", "", nil, ErrQPACKDecompressFailed
	}

	if malformed {
		return "", "", "", "", nil, ErrMalformedHeader
	}

	if method == "" || (method != "CONNECT" && (scheme == "" || path == "")) {
		return "", "", "", "", nil, ErrMissingMethodOrPath
	}

	return method, path, scheme, authority, headers, nil
}

// EncodeResponseHeaders encodes HTTP response status and headers into a QPACK-compressed byte slice.
func (q *QPACKCodec) EncodeResponseHeaders(statusCode int, headers http.Header, bodyLen int) []byte {
	pe := encoderPool.Get().(*PooledEncoder)
	defer encoderPool.Put(pe)

	pe.buf.Reset()
	pe.enc.Reset(pe.buf)

	// 1. :status pseudo-header
	_ = pe.enc.WriteField(qpack.HeaderField{
		Name:  ":status",
		Value: strconv.Itoa(statusCode),
	})

	// 2. content-length
	if bodyLen >= 0 {
		_ = pe.enc.WriteField(qpack.HeaderField{
			Name:  "content-length",
			Value: strconv.Itoa(bodyLen),
		})
	}

	// 3. Normal response headers
	for k, vv := range headers {
		if isForbiddenH3Header(k) {
			continue
		}

		kLower := strings.ToLower(k)
		for _, v := range vv {
			_ = pe.enc.WriteField(qpack.HeaderField{
				Name:  kLower,
				Value: v,
			})
		}
	}

	result := make([]byte, pe.buf.Len())
	copy(result, pe.buf.Bytes())

	return result
}

// isForbiddenH3Header returns true if the header is prohibited in HTTP/3 (RFC 9114 §4.2).
func isForbiddenH3Header(k string) bool {
	return bytesconv.EqualFoldASCII(k, "connection") ||
		bytesconv.EqualFoldASCII(k, "keep-alive") ||
		bytesconv.EqualFoldASCII(k, "proxy-connection") ||
		bytesconv.EqualFoldASCII(k, "transfer-encoding") ||
		bytesconv.EqualFoldASCII(k, "upgrade")
}
