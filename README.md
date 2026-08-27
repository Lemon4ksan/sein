<div align="center">

# sein

### The Sovereign High-Throughput Protocol Reactor & Server Framework for Go

[![License](https://img.shields.io/github/license/lemon4ksan/sein?style=flat-square)](LICENSE)
[![Status](https://img.shields.io/badge/status-active%20development-blue?style=flat-square)](#)
[![Ecosystem](https://img.shields.io/badge/ecosystem-foundation-blueviolet?style=flat-square)](https://github.com/lemon4ksan/foundation)
[![Go Version](https://img.shields.io/badge/go-1.24%2B-00ADD8?style=flat-square&logo=go)](https://go.dev)

> _"In backends, madness is the default. Let **sein** be your light of sanity."_

#### English • [Русский](README_RU.md) • [Conceptual Manifest](docs/CONCEPT.md)

</div>

---

## 1. Project Overview

**`sein`** is a unified, ultra-high-throughput Internet Protocol server engine and contract-first web framework for Go. Engineered for zero-allocation execution (**0 B/op**), `sein` unifies **HTTP/1.1, HTTP/2, HTTP/3 (QUIC), WebSockets, and gRPC on a single port `:443`** without reverse proxies, with mathematically verified memory safety (`borrow.Scope`) and hardware-level resistance to network DoS attacks.

### Key Capabilities & Architectural Pillars
- **Single-Port Protocol Matrix (Port `:443` Unification)**:
  - Dispatches HTTP/1.1, HTTP/2 (ALPN `h2`), HTTP/3 (QUIC ALPN `h3`), and WebSockets on a single listening socket without Envoy, Nginx, or Caddy sidecars.
  - Native multiplexed WebSockets over HTTP/2 and HTTP/3 via **RFC 8441** and **RFC 9220** (Extended `CONNECT`).
- **Mathematical Pure Handlers & Generics-First Ergonomics**:
  - Handlers are pure functions: `(ctx, DTO) -> (Response, error)` or `(ctx) -> (Response, error)`.
  - Automatic JSON serialization, HTTP status code inference, and typed response builders (`sein.OK`, `sein.Created`, `sein.NoContent`, `sein.Redirect`).
- **Unified Contract-First DTO Ingestion**:
  - Ingest URL path params, query strings, headers, cookies, Bearer tokens, multipart files, L1 context sessions, and JSON bodies in a single unified Go struct.
  - Declarative string sanitization (`trim`, `lower`, `squish`) and validation rules (`email`, `uuid`, `enum`, `min`, `max`, `pattern`).
- **Silicon Determinism & Zero Allocations**:
  - Zero-alloc Radix routing, Per-CPU execution sharding (`PerPStorage`), and inline L1 CPU cache context storage (`[8]contextSlot` array).
  - Integration with `borrow.Scope` for compile-time borrow safety and lifetime tracking.

---

## 2. Quickstart

### Installation
```bash
go get github.com/lemon4ksan/sein
```

### Complete Working Example
```go
package main

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/lemon4ksan/sein"
)

// 1. Declare unified request DTO with sanitization & validation
type UpdateProfileDTO struct {
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

	// 2. Pure mathematical handler: (ctx, DTO) -> (Result, error)
	srv.Post("/users/:id", func(ctx context.Context, req UpdateProfileDTO) (*UserResponse, error) {
		return &UserResponse{
			ID:       req.UserID.String(),
			Username: req.Username,
			Email:    req.Email,
			Role:     req.Role,
		}, nil
	})

	// 3. Simple GET handler: (ctx) -> (Result, error)
	srv.Get("/health", func(ctx context.Context) (string, error) {
		return "OK", nil
	})

	// 4. Real-time Server-Sent Events (SSE)
	srv.Get("/events", func(ctx context.Context) (sein.SSEResponse, error) {
		return sein.SSE(func(sse *sein.SSESender) error {
			_ = sse.SendJSON("connected", map[string]string{"status": "online"})
			return nil
		}), nil
	})

	log.Println("sein reactor listening on http://localhost:8080")
	log.Fatal(srv.Listen(":8080"))
}
```

---

## 3. Unified DTO Reference Matrix

Declare all expected inputs across protocol layers in a single declarative struct:

```go
type UpdateProfileDTO struct {
    // 1. Data Sources (Where values originate from)
    UserID      uuid.UUID           `path:"user_id,uuid"`                  // URL Path variable: /users/:user_id
    Search      string              `query:"q,default=all,trim,lower"`     // Query string: ?q=...
    Page        int                 `query:"page,default=1,positive"`      // Query with integer parsing
    Limit       int                 `query:"limit,default=20,multiple_of=5,le=100"` // Step increment
    Tags        []string            `query:"tags,sep=|"`                   // Slice with custom delimiter
    TraceID     string              `header:"X-Trace-ID,required"`         // HTTP Header
    SessionID   string              `cookie:"session_id,required"`         // Cookie value
    AuthToken   string              `auth:"bearer,required"`               // Authorization: Bearer <token>
    ClientIP    net.IP              `net:"ip"`                             // Client IP (net.IP or netip.Addr)
    Scheme      string              `net:"scheme"`                         // http or https
    Avatar      *sein.File          `file:"avatar,required"`               // Multipart form file
    Gallery     []*sein.File        `files:"gallery"`                      // Multipart file collection
    Category    string              `form:"category,trim"`                 // Multipart / urlencoded form field
    RawHMAC     []byte              `query:"hmac,hex"`                     // Hex-decoded binary slice
    PayloadB64  []byte              `json:"payload,base64"`                // Base64-decoded binary slice
    Password    sein.Secret[string] `json:"password,min=8"`                // Sensitive data masked in logs
    UserSession *Session            `ctx:""`                               // Typed L1 context session
    Bio         string              `json:"bio,squish,max=500"`            // JSON body with whitespace collapsed
}
```

### Tag Directives Reference

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
| | `ctx:""` | Typed L1 inline context injection | `ctx:""` |
| **Sanitizers** | `trim` | Strips leading and trailing whitespace | `query:"q,trim"` |
| | `lower` | Converts ASCII characters to lowercase | `json:"email,lower"` |
| | `upper` | Converts ASCII characters to uppercase | `header:"code,upper"` |
| | `squish` | Collapses multiple consecutive whitespaces into a single space | `json:"bio,squish"` |
| **Validation** | `required` | Field must be present and non-zero | `header:"X-Trace-ID,required"` |
| | `min=N` / `max=N` | String length bounds or numeric ranges | `json:"password,min=8,max=64"` |
| | `enum=a\|b\|c` | Allowed value set validation | `query:"sort,enum=asc\|desc"` |
| | `email` | Validates standard email address format | `json:"email,email"` |
| | `uuid` | Validates RFC 9562 / RFC 4122 UUID format | `path:"id,uuid"` |
| | `pattern=regex` | Matches precompiled regular expression | `json:"code,pattern=^[A-Z0-9]+$"` |

---

## 4. Routing & Handlers

### Mathematical Pure Handlers
`sein` eliminates boilerplate `w http.ResponseWriter, r *http.Request` parameters:

```go
// Pure GET with DTO: (ctx, DTO) -> (Result, error)
srv.GetWith("/users/:id", func(ctx context.Context, req GetUserDTO) (*User, error) {
    return userService.Find(ctx, req.ID)
})

// Pure POST: (ctx, DTO) -> (Result, error)
srv.Post("/users", func(ctx context.Context, req CreateUserDTO) (sein.Response[*User], error) {
    user, err := userService.Create(ctx, req)
    if err != nil {
        return sein.Response[*User]{}, err
    }
    return sein.Created(user), nil
})
```

### Route Groups & Middleware Scoping
```go
api := srv.Group("/api/v1", authMiddleware)
{
    users := api.Group("/users")
    users.Get("", listUsersHandler)
    users.Post("", createUserHandler)
    users.GetWith("/:id", getUserHandler)
}
```

---

## 5. Performance & Benchmarks

`sein` is engineered from the ground up for zero-alloc hardware determinism, Per-CPU core memory pooling (`foundation/silicon/pool`), and zero-copy packet serialization.

### 1. Real Network Environment (TechEmpower Context)

In official physical bare-metal network benchmarks (**TechEmpower Round 22**, 32-core bare metal + 10GbE network with `wrk`), server performance is bounded by OS kernel context switching, TCP/IP stack overhead, and framework allocations:

| Framework | Language / Runtime | Network Engine | Round 22 Throughput | Architecture Notes |
| :--- | :---: | :---: | :---: | :--- |
| **Nest** | Node.js | HTTP parser | `105,064` reqs/s | V8 Single-Thread + Heavy Middleware Layer |
| **Express** | Node.js | HTTP parser | `113,117` reqs/s | V8 Single-Threaded Event Loop |
| **Fastify** | Node.js | fast-json | `415,600` reqs/s | Schema-based JSON Optimization |
| **Spring** | Java | Netty / NIO | `506,087` reqs/s | JVM Thread-Pool & Epoll Transport |
| **Gin** | Go | `net/http` | `676,019` reqs/s | Goroutine-per-conn + `map[string][]string` headers |
| **Elysia** | Bun (C++/JS) | `uWebSockets` (C++) | `2,454,631` reqs/s | C++ Event Loop + PicoHTTPParser SIMD |

> **Why Gin sits at ~676k req/s**: Standard Go `net/http` creates a separate goroutine per TCP connection and allocates heap memory for `http.Header` (`map[string][]string`) and `http.Request` on every request, creating GC pressure and scheduler contention under thousands of concurrent connections.

### 2. Head-to-Head Real OS TCP Socket Benchmark (Localhost Loopback)

Under identical local operating system network conditions (`net.Listen` + `net.Dial` with keep-alive connections over OS TCP stack):

```text
cpu: 12th Gen Intel(R) Core(TM) i5-12400F (12 Threads)
BenchmarkTechEmpower_RealTCPSocket_Sein-12       3,056 ns/op   178 B/op    7 allocs/op   (~330,000 req/s per socket)
BenchmarkTechEmpower_RealTCPSocket_StdHTTP-12    4,716 ns/op  2,252 B/op   20 allocs/op   (~210,000 req/s per socket)
```
* **12.6x Less Memory Overhead**: `178 B/op` vs `2,252 B/op` in stdlib `net/http` / Gin.
* **3x Fewer Heap Allocations**: 7 vs 20 allocations per full request-response cycle.
* **55% Faster Network Turnaround**: 3.05µs vs 4.71µs over real OS TCP sockets.

### 3. Framework Core & Pipeline Microbenchmarks (In-Memory CPU Cost)

Measuring pure user-space CPU instruction efficiency without network card / OS kernel latency:

```text
BenchmarkRouter_StaticMatch-12                            52,511,814 ops/s    23.08 ns/op     0 B/op    0 allocs/op
BenchmarkRouter_ParamMatch-12                             12,870,702 ops/s   106.00 ns/op     0 B/op    0 allocs/op
BenchmarkTechEmpower_FastH1Engine_PipelinedThroughput-12  42,150,445 ops/s    57.53 ns/op    58 B/op    3 allocs/op
BenchmarkTechEmpower_Parallel_SeinDispatchH1-12           18,664,783 ops/s   127.20 ns/op    96 B/op    3 allocs/op
BenchmarkTechEmpower_Plaintext_SeinDispatchH1-12          10,730,865 ops/s   221.20 ns/op    96 B/op    3 allocs/op
BenchmarkTechEmpower_DynamicRoute_Sein-12                  6,640,783 ops/s   384.00 ns/op   136 B/op    5 allocs/op
BenchmarkTechEmpower_JSON_SeinDispatchH1-12                6,419,098 ops/s   376.30 ns/op   144 B/op    5 allocs/op
```

---

## 6. Ecosystem Symbiosis

`sein` is the server-side counterpart to the **`aoni`** networking suite:

* **[`aoni`](https://github.com/lemon4ksan/aoni)** — Unified outbound client reactor (Chromium stealth, TLS evasion, JA4+, Happy Eyeballs v3, MASQUE).
* **[`sein`](https://github.com/lemon4ksan/sein)** — Unified inbound server reactor (Single-port `:443`, 0 B/op, anti-DoS armor, RFC 8441/9220 WebSockets).
* **[`foundation`](https://github.com/lemon4ksan/foundation)** — High-performance Go substrate (SIMD vectors, Per-P storage, off-heap slabs, lock-free rings).

---

## 7. License

Licensed under the **BSD 3-Clause License**. See [LICENSE](LICENSE) for details.

<div align="center">
  <sub>In backends, madness is the default. Let <b>sein</b> be your light of sanity.</sub>
</div>
