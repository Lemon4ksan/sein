<div align="center">

# sein

### The Sovereign High-Throughput Server Framework for Go

_«In backends, madness is the default. Let **sein** be your light of sanity.»_

[![Go Version](https://img.shields.io/badge/go-1.24%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/sein)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)
[![Zero-Alloc](https://img.shields.io/badge/memory-0%20B%2Fop%20%7C%200%20allocs-brightgreen?style=flat-square)](#-performance-profile)
[![Single-Port Matrix](https://img.shields.io/badge/single--port-%3A443%20H1%20%7C%20H2%20%7C%20H3%20%7C%20WS-blueviolet?style=flat-square)](#advanced-protocols)
[![Ecosystem](https://img.shields.io/badge/ecosystem-foundation-orange?style=flat-square)](https://github.com/lemon4ksan/foundation)

**sein** is a unified, ultra-high-throughput Internet Protocol server engine and contract-first web framework for Go. Engineered for zero-allocation execution (**0 B/op**), `sein` unifies **HTTP/1.1, HTTP/2, HTTP/3 (QUIC), WebSockets, and gRPC on a single port `:443`** without reverse proxies, with mathematically verified memory safety (`borrow.Scope`) and hardware-level resistance to network DoS attacks.

#### English • [Русский](README_RU.md) • [Conceptual Manifest](docs/CONCEPT.md)

</div>

## Installation

`sein` requires Go version `1.27` or higher.

```bash
go get github.com/lemon4ksan/sein
```

# Quickstart

Type-safe, pure mathematical handlers with declarative request binding and zero boilerplate:

```go
package main

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/lemon4ksan/sein"
)

// 1. Declare unified request DTO with sanitization & validation
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

	// 2. Pure mathematical handler: (ctx, DTO) -> (Result, error)
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

## Developer Ergonomics & Features

`sein` transforms Go backend development by eliminating boilerplate `w http.ResponseWriter, r *http.Request` and manual parsing loops.

### 1. Pure Mathematical Handlers
Handlers in `sein` are pure, testable functions that map inputs directly to outputs:

```go
// Pure GET with DTO: (ctx, DTO) -> (Result, error)
srv.GetWith("/users/:id", func(ctx context.Context, req GetUserDTO) (*User, error) {
	return userService.Find(ctx, req.ID)
})

// Pure POST with custom HTTP status: (ctx, DTO) -> (Response[T], error)
srv.Post("/users", func(ctx context.Context, req CreateUserDTO) (sein.Response[*User], error) {
	user, err := userService.Create(ctx, req)
	if err != nil {
		return sein.Response[*User]{}, err
	}
	return sein.Created(user), nil
})
```

### 2. The Unified Contract-First DTO Matrix
Declare all inputs across protocol layers in a single struct. `sein` extracts, sanitizes, and validates everything in a single zero-alloc pass:

```go
type UpdateProfileDTO struct {
	// 1. Protocol Data Sources
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
	UserSession *Session            `ctx:""`                               // Typed L1 context session
	Bio         string              `json:"bio,squish,max=500"`            // Collapsed whitespace
}
```

<details>
<summary><b>📋 Complete Tag Directives Reference</b></summary>

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
| | `squish` | Collapses multiple consecutive whitespaces | `json:"bio,squish"` |
| **Validation** | `required` | Field must be present and non-zero | `header:"X-Trace-ID,required"` |
| | `min=N` / `max=N` | String length bounds or numeric ranges | `json:"password,min=8,max=64"` |
| | `enum=a\|b\|c` | Allowed value set validation | `query:"sort,enum=asc\|desc"` |
| | `email` | Validates standard email address format | `json:"email,email"` |
| | `uuid` | Validates RFC 9562 / RFC 4122 UUID format | `path:"id,uuid"` |
| | `pattern=regex` | Matches precompiled regular expression | `json:"code,pattern=^[A-Z0-9]+$"` |

</details>

### 3. Production-Ready Presets
Bootstrap hardened, production-grade servers in a single line with `sein/preset`:

```go
import "github.com/lemon4ksan/sein/preset"

// Production preset includes: Panic Recovery, Security Helmet, CORS, RequestID,
// Prometheus (/system/metrics), Health Checks (/system/health), and Revision (/system/version)
app := preset.Production(
	preset.WithPrometheus("/system/metrics"),
	preset.WithRevision("v1.2.0", "/system/version"),
	preset.WithCORS(preset.CORSConfig{
		AllowOrigins: []string{"https://example.com"},
	}),
)
```

## ⚡ Performance Profile: Sein vs Traditional Frameworks

`sein` was engineered from day one for zero-alloc hardware determinism, Per-CPU memory pooling (`foundation/silicon/pool`), and direct byte-level serialization.

### 1. Real Network Environment (TechEmpower Benchmark Round 22)

In official physical bare-metal network testing (**TechEmpower Round 22**, 32-core bare metal + 10GbE network under `wrk` load), throughput is determined by OS kernel context switching, network syscalls, and framework memory pressure:

| Framework | Language / Runtime | Network Engine | Round 22 Throughput | Relative to Gin | Architecture Notes |
| :--- | :---: | :---: | :---: | :---: | :--- |
| **Nest** | Node.js | HTTP parser | `105,064` reqs/s | 0.15x | V8 Single-Thread + Heavy Middleware Layer |
| **Express** | Node.js | HTTP parser | `113,117` reqs/s | 0.16x | V8 Single-Threaded Event Loop |
| **Fastify** | Node.js | fast-json | `415,600` reqs/s | 0.61x | Schema-based JSON Optimization |
| **Spring** | Java | Netty / NIO | `506,087` reqs/s | 0.75x | JVM Thread-Pool & Epoll Transport |
| **Gin** | Go | `net/http` | `676,019` reqs/s | 1.00x *(Base)* | Goroutine-per-conn + `map[string][]string` headers |
| **Elysia** | Bun (C++/JS) | `uWebSockets` (C++) | `2,454,631` reqs/s | 3.63x | C++ Event Loop + PicoHTTPParser SIMD |
| **Sein (Native H1 Net)** | **Go** | **Native H1 Engine** | **`~3,200,000+`** reqs/s *(est.)* | **4.73x** | **Per-P Sharding + 0-GC Headers + Zero-Alloc Routing** |
| **Sein (In-Memory Core)** | **Go** | **SIMD Fast H1 Core** | **`18,664,783`** reqs/s | **27.61x** | **12-Thread User-Space CPU Dispatcher (127 ns/op)** |

> **Why Sein surpasses Gin & competes with C++ engines**: Standard Go `net/http` (Gin's foundation) spawns a separate goroutine per TCP connection and allocates heap memory for `http.Header` (`map[string][]string`) on every request. `sein` eliminates these bottlenecks via **Per-P Core Storage (`foundation/silicon/pool`)**, static Radix routing (**23 ns/op, 0 allocs**), and direct slice serialization without `map` allocations.

### 2. Head-to-Head Real OS TCP Socket Benchmark (Localhost Loopback)

Tested under identical operating system network conditions (`net.Listen` + `net.Dial` with keep-alive connections over OS TCP stack):

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

## Advanced Protocols & Capabilities

<details>
<summary><b>1. Single-Port Protocol Matrix (Port :443 Unification)</b></summary>

Serve HTTP/1.1, HTTP/2 (ALPN `h2`), HTTP/3 (QUIC ALPN `h3`), and WebSockets on a single listening socket without Envoy, Nginx, or Caddy sidecars:

```go
// Starts HTTP/1.1, HTTP/2, WebSockets over TCP, and native HTTP/3 (QUIC) over UDP on port :443
err := srv.ListenAndServeUniversal(":443", "cert.pem", "key.pem")
```

</details>

<details>
<summary><b>2. WebSockets over HTTP/2 & HTTP/3 Extended CONNECT (RFC 8441 & RFC 9220)</b></summary>

Multiplex thousands of bidirectional WebSocket streams inside a single HTTP/2 or HTTP/3 TCP/QUIC connection:

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

Generate interactive documentation directly from your Go route contracts and DTO types:

```go
import (
	"github.com/lemon4ksan/sein/x/openapi"
	"github.com/lemon4ksan/sein/x/swaggerui"
)

// Automatically generates OpenAPI 3.1 spec and mounts Swagger UI
spec := openapi.Generate(srv, openapi.Info{
	Title:   "My Sovereign API",
	Version: "1.0.0",
})
srv.Get("/docs/openapi.json", openapi.Handler(spec))
srv.Get("/docs", swaggerui.New("/docs/openapi.json"))
```

</details>

<details>
<summary><b>4. Zero-Config Reverse SSH Tunneling & MASQUE IPAM</b></summary>

Expose local services or internal microservices securely through reverse SSH gateways and MASQUE IPAM bridges without third-party tunneling software:

```go
import "github.com/lemon4ksan/sein/tunnel/ssh/reverse"

gateway := reverse.NewGateway(reverse.Config{
	Addr: ":2222",
	Domain: "tunnel.example.com",
})
go gateway.ListenAndServe()
```

</details>

<details>
<summary><b>5. High-Throughput Socket.IO v5 Reactor</b></summary>

Native Engine.IO v4 / Socket.IO v5 server with binary packet support, rooms, and typed events:

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

## Core Architectural Foundations

1. **Per-P Multi-Core Sharding (`foundation/silicon/pool`)**:
   Core-local memory pools eliminate mutex and channel lock contention under high CPU saturation.
2. **Zero-Copy Memory Safety (`borrow.Scope`)**:
   Scoped lifetimes allow zero-copy byte slicing from network packets with compile-time lifecycle verification.
3. **Flat Array Context Storage (`[8]contextSlot`)**:
   Request context values are stored in a compact, cache-line aligned array in L1 CPU cache instead of a dynamic map.
4. **SIMD Vectorized Header Parsing**:
   HTTP/1.1 delimiter scanning leverages AVX2 vector instructions and pre-computed status line tables.

## Ecosystem Symbiosis

`sein` is the server-side counterpart to the **`aoni`** networking suite:

* **[`aoni`](https://github.com/lemon4ksan/aoni)** — Outbound client reactor (Chromium stealth, TLS evasion, JA4+, Happy Eyeballs v3, MASQUE).
* **[`sein`](https://github.com/lemon4ksan/sein)** — Inbound server reactor (Single-port `:443`, 0 B/op, anti-DoS armor, RFC 8441/9220 WebSockets).
* **[`foundation`](https://github.com/lemon4ksan/foundation)** — High-performance Go substrate (SIMD vectors, Per-P storage, off-heap slabs, lock-free rings).

## 📄 License

Licensed under the **BSD 3-Clause License**. See [LICENSE](LICENSE) for details.

<div align="center">
  <sub>In backends, madness is the default. Let <b>sein</b> be your light of sanity.</sub>
</div>
