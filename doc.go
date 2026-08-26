// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sein provides a high-performance, contract-first HTTP server framework for Go.
//
// # Overview
//
// Sein is designed around pure mathematical functions, zero-allocation radix routing,
// and single-contract DTO ingestion. Handlers declare all expected inputs (path, query,
// headers, cookies, auth tokens, client telemetry, multipart files, L1 context sessions,
// and JSON bodies) in a single unified struct.
//
// # Unified DTO Quick Reference
//
// A canonical example illustrating all available DTO binding sources, sanitizers, and validation rules:
//
//	type UpdateProfileDTO struct {
//	    // 1. Data Sources (Where values originate from)
//	    UserID      uuid.UUID           `path:"user_id,uuid"`                  // URL Path variable: /users/:user_id
//	    Search      string              `query:"q,default=all,trim,lower"`     // Query string: ?q=...
//	    Page        int                 `query:"page,default=1,positive"`      // Query with integer parsing
//	    Limit       int                 `query:"limit,default=20,multiple_of=5,le=100"` // Step increment
//	    Tags        []string            `query:"tags,sep=|"`                   // Slice with custom delimiter
//	    TraceID     string              `header:"X-Trace-ID,required"`         // HTTP Header
//	    SessionID   string              `cookie:"session_id,required"`         // Cookie value
//	    AuthToken   string              `auth:"bearer,required"`               // Authorization: Bearer <token>
//	    ClientIP    net.IP              `net:"ip"`                             // Client IP (net.IP or netip.Addr)
//	    Scheme      string              `net:"scheme"`                         // http or https
//	    Avatar      *sein.File          `file:"avatar,required"`               // Multipart form file
//	    Gallery     []*sein.File        `files:"gallery"`                      // Multipart file collection
//	    Category    string              `form:"category,trim"`                 // Multipart / urlencoded form field
//	    RawHMAC     []byte              `query:"hmac,hex"`                     // Hex-decoded binary slice
//	    PayloadB64  []byte              `json:"payload,base64"`                // Base64-decoded binary slice
//	    Password    sein.Secret[string] `json:"password,min=8"`                // Sensitive data masked in logs
//	    UserSession *Session            `ctx:""`                               // Typed L1 context session
//	    Bio         string              `json:"bio,squish,max=500"`            // JSON body with whitespace collapsed
//	}
//
// # Tag Directives Reference
//
// 1. Sources:
//   - `path:"key"` or `param:"key"`: URL path parameter (e.g. /users/:id)
//   - `query:"key"`: URL query parameter
//   - `header:"key"`: HTTP request header
//   - `cookie:"key"`: HTTP cookie
//   - `auth:"bearer"`: Authorization Bearer token
//   - `net:"ip"` / `net:"proto"` / `net:"scheme"` / `net:"host"` / `net:"method"` / `net:"path"`: Telemetry
//   - `form:"key"`: Form field
//   - `file:"key"`: Single multipart uploaded file (*sein.File)
//   - `files:"key"`: Multiple multipart uploaded files ([]*sein.File)
//   - `body:"raw"` / `body:"string"`: Raw request body ([]byte or string)
//   - `ctx:""` / `context:""`: L1 typed request context injection
//   - `json:"key"`: JSON body payload field
//
// 2. Modifiers & Options:
//   - `required`: Field must be present and non-empty
//   - `default=value`: Fallback value when parameter is missing or empty
//   - `format="layout"`: Custom timestamp layout for time.Time fields
//   - `sep="delimiter"`: Custom slice element separator (default is ",")
//
// 3. String Sanitizers (Executed sequentially before validation):
//   - `trim`: Strips leading and trailing whitespace
//   - `lower`: Converts ASCII characters to lowercase
//   - `upper`: Converts ASCII characters to uppercase
//   - `single_space` / `squish`: Replaces consecutive whitespace runs with a single space
//   - `digits_only`: Strips all non-digit characters
//
// 4. Declarative Validation Rules:
//   - `min=N` / `max=N`: Minimum / maximum string length or numeric value
//   - `len=N`: Exact string length
//   - `gt=N` / `ge=N` / `lt=N` / `le=N`: Strict numeric inequalities
//   - `positive` / `negative` / `non_negative`: Numeric sign predicates
//   - `multiple_of=N`: Enforces that numeric value is divisible by N
//   - `enum=a|b|c`: Value must match one of the pipe-separated allowed options
//   - `pattern=regex`: Precompiled regular expression match
//   - `email`: Validates standard email address format
//   - `uuid`: Validates RFC 9562 / RFC 4122 UUID format
//   - `url`: Validates absolute URL format
//   - `hex`: Decodes hex-encoded string into []byte
//   - `base64`: Decodes base64-encoded string into []byte
//
// 5. Custom Domain Validation:
//
// If a DTO struct implements the Validatable interface, its Validate() error method
// is automatically invoked after all declarative validations pass:
//
//	func (d *UpdateProfileDTO) Validate() error {
//	    if d.Search == "" && d.Bio == "" {
//	        return errors.New("at least one of search or bio must be provided")
//	    }
//	    return nil
//	}
package sein
