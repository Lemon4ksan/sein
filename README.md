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

**sein** is a server network stack and web framework for Go. It supports running HTTP/1.1, HTTP/2, HTTP/3 (QUIC), WebSockets, and gRPC on a single port `:443` without reverse proxies, with universal handler compilation, contract-first DTO binding, and table-driven domain error mapping.

#### English • [Русский](README_RU.md) • [Architecture Concept](docs/CONCEPT.md)

</div>

## Installation

`sein` requires Go version `1.27` or higher.

```bash
go get github.com/lemon4ksan/sein
```

## Quickstart

Universal handlers, declarative validation, and zero-glue domain routing:

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
	UserID   uuid.UUID `path:"id" validate:"uuid"`
	Username string    `json:"username" validate:"required,min=3,max=30" sanitize:"trim"`
	Email    string    `json:"email" validate:"required,email" sanitize:"lower"`
	Role     string    `query:"role,default=user" validate:"enum=user|admin|moderator"`
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

	// 2. Universal Handlers: pass pure Go functions directly
	srv.Get("/health", func(ctx context.Context) (string, error) {
		return "OK", nil
	})

	srv.Post("/users/:id", func(ctx context.Context, req UpdateUserDTO) (*UserResponse, error) {
		return &UserResponse{
			ID:       req.UserID.String(),
			Username: req.Username,
			Email:    req.Email,
			Role:     req.Role,
		}, nil
	})

	// 3. Server-Sent Events (SSE)
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

## Universal Routing & Zero-Glue Architecture

`sein` features a universal handler compiler: standard HTTP verbs (`Get`, `Post`, `Patch`, `Delete`, `Put`) accept any pure Go function signature without requiring framework-specific glue wrappers.

### 1. Supported Handler Signatures

| Purpose | Handler Signature | Data Extraction | Return Value |
| :--- | :--- | :--- | :--- |
| **Action** | `func(ctx context.Context) error` | None (context only) | `200 OK` on `nil` |
| **Query** | `func(ctx context.Context) (Res, error)` | None (context only) | JSON response |
| **Direct Path ID** | `func(ctx context.Context, id ID) (Res, error)` | URL parameter `:id` (Snowflake, uint64, string, UUID) | JSON response |
| **Path ID Action** | `func(ctx context.Context, id ID) error` | URL parameter `:id` | `200 OK` on `nil` |
| **DTO Payload** | `func(ctx context.Context, req DTO) (Res, error)` | DTO (JSON Body / Query / Headers) | JSON response |
| **ID + Body Payload** | `func(ctx context.Context, id ID, req DTO) (Res, error)` | `:id` from URL + JSON Body | JSON response |
| **Raw Request** | `func(req *sein.Request) (Res, error)` | Direct request access | JSON response |

### 2. Zero-Glue Controllers (Service Method Promotion)

Because `sein` handler signatures match standard domain service signatures, you can embed services into modules/controllers and mount methods directly:

```go
type BotController struct {
	*bots.Service // Auto-promotes Create, Get, Delete, Update, etc.
}

func (c *BotController) Mount(g *sein.Group) {
	// Table-driven domain error mapping
	g.MapErrors(sein.Errors{
		database.ErrNotFound:       ErrBotNotFound,
		bots.ErrInvalidUserID:      ErrInvalidBotUserID,
		bots.ErrActiveBot:          ErrBotActiveCannotDelete,
		bots.ErrAlreadyLinkedAccount: ErrBotAlreadyLinkedAccount,
	})

	// Direct service method binding with ZERO forwarding shims
	g.Post("", c.Create)
	g.Get("/:id", c.Get)
	g.Patch("/:id", c.Update)       // Takes (ctx, id Snowflake, payload UpdatePayload)
	g.Patch("/:id/type", c.SetType)
	g.Delete("/:id", c.Delete)
	g.Post("/:id/disconnect", c.Disconnect) // Takes (ctx, id Snowflake) error
}
```

### 3. Table-Driven Domain Error Mapping

Map internal sentinel errors to typed HTTP domain errors declaratively using `sein.Errors`:

```go
var (
	ErrUserNotFound = sein.NotFound("USER_NOT_FOUND", "User does not exist")
	ErrBusyEmail    = sein.Conflict("EMAIL_EXISTS", "Email is already taken")
)

users.MapErrors(sein.Errors{
	database.ErrNotFound:  ErrUserNotFound,
	users.ErrEmailTaken:   ErrBusyEmail,
})
```

## DTO Structs & Declarative Validation

Declare all request inputs (path, query, headers, cookies, JSON payload) in a unified DTO struct with automatic validation and sanitization:

```go
type UpdateProfileDTO struct {
	// Protocol Data Sources
	UserID      uuid.UUID           `path:"user_id" validate:"uuid"`       // URL Path: /users/:user_id
	Search      string              `query:"q,default=all" sanitize:"trim,lower"` // Query string: ?q=...
	Page        int                 `query:"page,default=1" validate:"positive"` // Query with integer parsing
	Limit       int                 `query:"limit,default=20" validate:"multiple_of=5,le=100"` // Step bounds
	Tags        []string            `query:"tags,sep=|"`                   // Slice with custom separator
	TraceID     string              `header:"X-Trace-ID" validate:"required"` // HTTP Header
	SessionID   string              `cookie:"session_id" validate:"required"` // HTTP Cookie
	AuthToken   string              `auth:"bearer,required"`               // Authorization: Bearer <token>
	ClientIP    net.IP              `net:"ip"`                             // Resolved Client IP
	Avatar      *sein.File          `file:"avatar,required"`               // Uploaded File
	Gallery     []*sein.File        `files:"gallery"`                      // Multiple Uploaded Files
	Password    sein.Secret[string] `json:"password" validate:"min=8"`     // Masked in logs & stack traces
	UserSession *Session            `ctx:""`                               // Typed context session
	Bio         string              `json:"bio" validate:"max=500" sanitize:"squish"` // Collapsed whitespace
}
```

<details>
<summary><b>📋 Tag Directives Reference</b></summary>

| Category | Directive | Description | Example |
| :--- | :--- | :--- | :--- |
| **Sources** | `path:"key"` | URL path parameter (`/users/:id`) | `path:"id"` |
| | `query:"key"` | URL query parameter (`?page=1`) | `query:"page,default=1"` |
| | `header:"key"` | HTTP request header | `header:"X-API-Key"` |
| | `cookie:"key"` | HTTP cookie value | `cookie:"session_id"` |
| | `auth:"bearer"` | Extracts `Authorization: Bearer <token>` | `auth:"bearer,required"` |
| | `form:"key"` | Form field value (multipart or urlencoded) | `form:"title"` |
| | `file:"key"` | Single uploaded multipart file (`*sein.File`) | `file:"avatar,required"` |
| | `files:"key"` | Multiple uploaded multipart files (`[]*sein.File`) | `files:"attachments"` |
| | `json:"key"` | JSON request body payload field | `json:"name"` |
| | `net:"ip"` | Resolved remote client IP address | `net:"ip"` |
| | `ctx:""` | Typed inline context injection | `ctx:""` |
| **Sanitizers (`sanitize:"..."`)** | `trim` | Strips leading and trailing whitespace | `sanitize:"trim"` |
| | `lower` | Converts ASCII characters to lowercase | `sanitize:"lower"` |
| | `upper` | Converts ASCII characters to uppercase | `sanitize:"upper"` |
| | `squish` | Collapses multiple consecutive whitespaces | `sanitize:"squish"` |
| | `digits_only` | Extracts digits only from string | `sanitize:"digits_only"` |
| **Validation (`validate:"..."`)** | `required` | Field must be present and non-zero | `validate:"required"` |
| | `min=N` / `max=N` | String length bounds or numeric ranges | `validate:"min=8,max=64"` |
| | `enum=a\|b\|c` | Allowed value set validation | `validate:"enum=asc\|desc"` |
| | `email` | Validates standard email address format | `validate:"email"` |
| | `uuid` | Validates UUID format (RFC 4122 / RFC 9562) | `validate:"uuid"` |
| | `pattern=regex` | Matches precompiled regular expression | `validate:"pattern=^[A-Z0-9]+$"` |

</details>

## Configuration Presets

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
| **Sein (Native H1 Net)** | **Go** | **Native H1 Engine** | **`~3,200,000+`**\* reqs/s | **4.73x** |
| **Sein (In-Memory Core)** | **Go** | **SIMD Fast H1 Core** | **`21,291,486`**\* reqs/s | **31.50x** |

> \* Local results. Not tested on an actual server.

### 2. OS TCP Socket Comparison (Loopback)

Tested over OS TCP stack with keep-alive connections (`net.Listen` + `net.Dial`):

```text
cpu: 12th Gen Intel(R) Core(TM) i5-12400F (12 Threads)
BenchmarkTechEmpower_RealTCPSocket_Sein-12       3,056 ns/op   178 B/op    7 allocs/op   (~330,000 req/s per socket)
BenchmarkTechEmpower_RealTCPSocket_StdHTTP-12    4,716 ns/op  2,252 B/op   20 allocs/op   (~210,000 req/s per socket)
```

## License

`sein` is distributed under the [BSD-3-Clause License](LICENSE).
