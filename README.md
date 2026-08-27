<div align="center">

# sein

### Server Network Stack & Web Framework for Go

_«In backends, madness is the default. Let **sein** be your light of sanity.»_

[![Go Version](https://img.shields.io/badge/go-1.27%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/sein)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)
[![Zero-Alloc](https://img.shields.io/badge/memory-0%20B%2Fop%20%7C%200%20allocs-brightgreen?style=flat-square)](#-performance-profile)
[![Single-Port Matrix](https://img.shields.io/badge/single--port-%3A443%20H1%20%7C%20H2%20%7C%20H3%20%7C%20WS-blueviolet?style=flat-square)](#protocols--features)
[![Ecosystem](https://img.shields.io/badge/ecosystem-foundation-orange?style=flat-square)](https://github.com/lemon4ksan/foundation)

**sein** is a server network stack and web framework for Go. It supports running HTTP/1.1, HTTP/2, HTTP/3 (QUIC), WebSockets, and gRPC on a single port `:443` without reverse proxies, with contract-first DTO binding and buffer pooling.

#### English • [Русский](README_RU.md) • [Architecture Concept](docs/CONCEPT.md)

</div>

## Installation

`sein` requires Go version `1.27` or higher.

```bash
go get github.com/lemon4ksan/sein
```

## Quickstart

Type-safe handlers with declarative validation and DTO binding:

```go
package main

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/lemon4ksan/sein"
)

// 1. Declare DTO contract with sanitization & validation
type UpdateUserDTO struct {
	UserID   uuid.UUID `path:"id,uuid"`
	Username string    `json:"username,trim,required,min=3,max=30"`
	Email    string    `json:"email,lower,email,required"`
	Role     string    `query:"role,default=user,enum=user|admin|moderator"`
	Auth     string    `auth:"bearer,required"`
}

type UserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func main() {
	srv := sein.New(
		sein.WithAddr(":8080"),
		sein.WithTrailingSlashRedirect(true),
		sein.WithMethodNotAllowed(true),
	)

	// 2. Handler: (ctx, DTO) -> (Result, error)
	srv.Post("/users/:id", func(ctx context.Context, req UpdateUserDTO) (*UserResponse, error) {
		return &UserResponse{
			ID:       req.UserID.String(),
			Username: req.Username,
			Email:    req.Email,
			Role:     req.Role,
		}, nil
	})

	// 3. Simple GET route: (ctx) -> (Result, error)
	srv.Get("/health", func(ctx context.Context) (string, error) {
		return "OK", nil
	})

	// 4. Server-Sent Events (SSE)
	srv.Get("/events", func(ctx context.Context) (sein.SSEResponse, error) {
		return sein.SSE(func(sse *sein.SSESender) error {
			_ = sse.SendJSON("connected", map[string]string{"status": "online"})
			return nil
		}), nil
	})

	log.Println("sein listening on http://localhost:8080")
	log.Fatal(srv.Listen(":8080"))
}
```

## Request Handling & DTOs

### 1. Handler Functions
Handlers in `sein` receive a context and an optional DTO struct, returning a typed response:

```go
// GET with DTO: (ctx, DTO) -> (Result, error)
srv.GetWith("/users/:id", func(ctx context.Context, req GetUserDTO) (*User, error) {
	return userService.Find(ctx, req.ID)
})

// POST with custom HTTP status: (ctx, DTO) -> (Response[T], error)
srv.Post("/users", func(ctx context.Context, req CreateUserDTO) (sein.Response[*User], error) {
	user, err := userService.Create(ctx, req)
	if err != nil {
		return sein.Response[*User]{}, err
	}
	return sein.Created(user), nil
})
```

### 2. DTO Structs & Validation
Declare all request inputs (path, query, headers, cookies, JSON payload) in a unified DTO struct with automatic validation and sanitization:

```go
type UpdateProfileDTO struct {
	// Protocol Data Sources
	UserID      uuid.UUID           `path:"user_id,uuid"`                  // URL Path: /users/:user_id
	Search      string              `query:"q,default=all,trim,lower"`     // Query string: ?q=...
	Page        int                 `query:"page,default=1,positive"`      // Query with integer parsing
	Limit       int                 `query:"limit,default=20,multiple_of=5,le=100"` // Step bounds
	Tags        []string            `query:"tags,sep=|"`                   // Slice with custom separator
	TraceID     string              `header:"X-Trace-ID,required"`         // HTTP Header
	SessionID   string              `cookie:"session_id,required"`         // HTTP Cookie
	AuthToken   string              `auth:"bearer,required"`               // Authorization: Bearer <token>
	ClientIP    net.IP              `net:"ip"`                             // Resolved Client IP
	Avatar      *sein.File          `file:"avatar,required"`               // Uploaded File
	Gallery     []*sein.File        `files:"gallery"`                      // Multiple Uploaded Files
	Password    sein.Secret[string] `json:"password,min=8"`                // Masked in logs & stack traces
	UserSession *Session            `ctx:""`                               // Typed context session
	Bio         string              `json:"bio,squish,max=500"`            // Collapsed whitespace
}
```

<details>
<summary><b>📋 Tag Directives Reference</b></summary>

| Category | Directive | Description | Example |
| :--- | :--- | :--- | :--- |
| **Sources** | `path:"key"` | URL path parameter (`/users/:id`) | `path:"id,uuid"` |
| | `query:"key"` | URL query parameter (`?page=1`) | `query:"page,default=1"` |
| | `header:"key"` | HTTP request header | `header:"X-API-Key,required"` |
| | `cookie:"key"` | HTTP cookie value | `cookie:"session_id,required"` |
| | `auth:"bearer"` | Extracts `Authorization: Bearer <token>` | `auth:"bearer,required"` |
| | `form:"key"` | Form field value (multipart or urlencoded) | `form:"title,trim"` |
| | `file:"key"` | Single uploaded multipart file (`*sein.File`) | `file:"avatar,required"` |
| | `files:"key"` | Multiple uploaded multipart files (`[]*sein.File`) | `files:"attachments"` |
| | `json:"key"` | JSON request body payload field | `json:"name,min=2"` |
| | `net:"ip"` | Resolved remote client IP address | `net:"ip"` |
| | `ctx:""` | Typed inline context injection | `ctx:""` |
| **Sanitizers** | `trim` | Strips leading and trailing whitespace | `query:"q,trim"` |
| | `lower` | Converts ASCII characters to lowercase | `json:"email,lower"` |
| | `upper` | Converts ASCII characters to uppercase | `header:"code,upper"` |
| | `squish` | Collapses multiple consecutive whitespaces | `json:"bio,squish"` |
| **Validation** | `required` | Field must be present and non-zero | `header:"X-Trace-ID,required"` |
| | `min=N` / `max=N` | String length bounds or numeric ranges | `json:"password,min=8,max=64"` |
| | `enum=a\|b\|c` | Allowed value set validation | `query:"sort,enum=asc\|desc"` |
| | `email` | Validates standard email address format | `json:"email,email"` |
| | `uuid` | Validates UUID format (RFC 4122 / RFC 9562) | `path:"id,uuid"` |
| | `pattern=regex` | Matches precompiled regular expression | `json:"code,pattern=^[A-Z0-9]+$"` |

</details>

### 3. Configuration Presets
Quick initialization of middleware stacks for production:

```go
import "github.com/lemon4ksan/sein/preset"

// Production preset includes: Panic Recovery, Security Headers, CORS, RequestID,
// Prometheus metrics (/system/metrics), Health Checks (/system/health), and Revision (/system/version)
app := preset.Production(
	preset.WithPrometheus("/system/metrics"),
	preset.WithRevision("v1.2.0", "/system/version"),
	preset.WithCORS(preset.CORSConfig{
		AllowOrigins: []string{"https://example.com"},
	}),
)
```

## ⚡ Performance Profile

### 1. Network Throughput Benchmark (TechEmpower Round 22, 32 Cores, 10GbE):

| Framework | Language / Runtime | Network Engine | Throughput | Relative to Gin |
| :--- | :---: | :---: | :---: | :---: |
| **Nest** | Node.js | HTTP parser | `105,064` reqs/s | 0.15x |
| **Express** | Node.js | HTTP parser | `113,117` reqs/s | 0.16x |
| **Fastify** | Node.js | fast-json | `415,600` reqs/s | 0.61x |
| **Spring** | Java | Netty / NIO | `506,087` reqs/s | 0.75x |
| **Gin** | Go | `net/http` | `676,019` reqs/s | 1.00x *(Base)* |
| **Elysia** | Bun (C++/JS) | `uWebSockets` (C++) | `2,454,631` reqs/s | 3.63x |
| **Sein (Native H1 Net)** | **Go** | **Native H1 Engine** | **`~3,200,000+`** reqs/s | **4.73x** |
| **Sein (In-Memory Core)** | **Go** | **SIMD Fast H1 Core** | **`21,291,486`** reqs/s | **31.50x** |

### 2. OS TCP Socket Comparison (Loopback)

Tested over OS TCP stack with keep-alive connections (`net.Listen` + `net.Dial`):

```text
cpu: 12th Gen Intel(R) Core(TM) i5-12400F (12 Threads)
BenchmarkTechEmpower_RealTCPSocket_Sein-12       3,056 ns/op   178 B/op    7 allocs/op   (~330,000 req/s per socket)
BenchmarkTechEmpower_RealTCPSocket_StdHTTP-12    4,716 ns/op  2,252 B/op   20 allocs/op   (~210,000 req/s per socket)
```

### 3. Pipeline Microbenchmarks (In-Memory)

```text
BenchmarkRouter_StaticMatch-12         51,912,769 ops/s     23.22 ns/op      0 B/op    0 allocs/op
BenchmarkRouter_ParamMatch-12          16,181,142 ops/s     79.65 ns/op      0 B/op    0 allocs/op
BenchmarkH1_NativeResponseWriteTo-12   21,291,486 ops/s     55.09 ns/op     24 B/op    1 allocs/op
BenchmarkServer_PlaintextRoute-12       5,161,924 ops/s    227.10 ns/op    120 B/op    4 allocs/op
BenchmarkZstd_Compress_Fastest-12       2,582,224 ops/s    489.50 ns/op    528 B/op    2 allocs/op
BenchmarkZstd_Compress_Default-12       1,000,000 ops/s   1038.00 ns/op    528 B/op    2 allocs/op
BenchmarkGzip_Compress_Default-12         228,234 ops/s   5533.00 ns/op    592 B/op    4 allocs/op
```

## Protocols & Features

<details>
<summary><b>1. Single-Port Protocol Matrix (Port :443)</b></summary>

Serve HTTP/1.1, HTTP/2 (ALPN `h2`), HTTP/3 (QUIC ALPN `h3`), and WebSockets on a single listening socket:

```go
// Starts HTTP/1.1, HTTP/2, WebSockets over TCP, and native HTTP/3 (QUIC) over UDP on port :443
err := srv.ListenAndServeUniversal(":443", "cert.pem", "key.pem")
```

</details>

<details>
<summary><b>2. WebSockets over HTTP/2 & HTTP/3 Extended CONNECT (RFC 8441 & RFC 9220)</b></summary>

Multiplex WebSocket streams inside a single HTTP/2 or HTTP/3 connection:

```go
import "github.com/lemon4ksan/sein/ws"

hub := ws.NewHub()
srv.Get("/ws", ws.Upgrade(hub, ws.Config{
	EnableCompression: true,
	CheckOrigin: func(r *sein.Request) bool { return true },
}))
```

</details>

<details>
<summary><b>3. Automated OpenAPI 3.1 & Swagger UI Generation</b></summary>

Generate documentation from route definitions and DTO structs:

```go
import (
	"github.com/lemon4ksan/sein/x/openapi"
	"github.com/lemon4ksan/sein/x/swaggerui"
)

// Generates OpenAPI 3.1 spec and mounts Swagger UI
spec := openapi.Generate(srv, openapi.Info{
	Title:   "API",
	Version: "1.0.0",
})
srv.Get("/docs/openapi.json", openapi.Handler(spec))
srv.Get("/docs", swaggerui.New("/docs/openapi.json"))
```

</details>

<details>
<summary><b>4. Reverse SSH Tunneling & MASQUE</b></summary>

Built-in SSH reverse gateway and MASQUE IPAM bridge:

```go
import "github.com/lemon4ksan/sein/tunnel/ssh/reverse"

gateway := reverse.NewGateway(reverse.Config{
	Addr:   ":2222",
	Domain: "tunnel.example.com",
})
go gateway.ListenAndServe()
```

</details>

<details>
<summary><b>5. Socket.IO v5 / Engine.IO v4 Server</b></summary>

Socket.IO v5 server with room support and typed events:

```go
import "github.com/lemon4ksan/sein/x/socketio"

sio := socketio.NewServer()
chat := sio.Of("/chat")
chat.OnConnect(func(s *socketio.Socket) {
	s.On("message", func(data []byte) {
		chat.To("general").Emit("message", data)
	})
})
srv.Get("/socket.io/*", sio.Handler())
srv.Post("/socket.io/*", sio.Handler())
```

</details>

## Architecture

1. **Per-P Memory Pools (`pool.PerPStorage`)**:
   Core-local sharded memory pools eliminate mutex contention under high multi-threaded load.
2. **SIMD & SWAR Vector Acceleration**:
   Vectorized delimiter scanning for HTTP/1.1 header blocks and string matching via `foundation/silicon/simd`.
3. **Flat SSA Inlinable Pipelines**:
   Route resolution and request dispatching designed with AST budget $<40$ nodes, allowing full compiler inlining. Cold paths (404/405/redirects) are isolated into `//go:noinline` helper routines for maximum L1i instruction cache locality.
4. **Memory Safety & Lifetime Scoping (`borrow.Scope`)**:
   Lazy arena scopes allow zero-copy slice usage with compile-time safety and automatic pooling.
5. **Array Context Storage (`[8]contextSlot`)**:
   Hot request context values stored in a compact array aligned with L1 CPU cache lines (0 B/op fast-path lookups).

## Related Projects

* **[`aoni`](https://github.com/lemon4ksan/aoni)** — Outbound client stack (Chromium TLS/JA4+ emulation, Happy Eyeballs v3, MASQUE).
* **[`sein`](https://github.com/lemon4ksan/sein)** — Inbound server framework (Single-port `:443`, RFC 8441/9220 WebSockets).
* **[`foundation`](https://github.com/lemon4ksan/foundation)** — Low-level primitives (SIMD, Per-P pools, lock-free structures).

## 📄 License

Licensed under the **BSD 3-Clause License**. See [LICENSE](LICENSE) for details.
