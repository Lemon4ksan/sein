# sein — The Philosophy & Architecture

```
                  ┌─────────────────────────────────────────┐
                  │                 S E I N                 │
                  │   /zaɪn/ — Pure Being & Static Order    │
                  └─────────────────────────────────────────┘
```

> *«The moment incoming bytes hit the network socket — they are processed with 0 allocations via lexical bump arenas, scanned at hardware SIMD line speed, and dispatched into pure typed domain functions on a single unified multi-protocol port.»*

## 1. The Core Philosophy: Elimination of Framework Glue

For over a decade, the Go backend ecosystem has been trapped in the **Context-Mutation Anti-Pattern** (`c *gin.Context`, `c *fiber.Ctx`). Developers were forced to write repetitive glue:
* Manually parsing DTOs with error-checking boilerplate.
* Mutating opaque framework contexts via `c.Set()` and `c.Get()`.
* Ending request chains with framework side-effects (`c.JSON(200, res)`).
* Wrestling with mock recorders and fake servers just to unit-test a simple endpoint.

**`sein` destroys this paradigm.**

In `sein`, your HTTP, WebSocket, RPC, and gRPC endpoints are **Pure Mathematical Functions of your Domain Model**:

```go
func (s *OrderService) CreateOrder(ctx context.Context, id Snowflake, req CreateOrderDTO) (*OrderResponse, error)
```

There is **zero framework lock-in** inside your business logic. A handler does not know whether it was invoked by an HTTP/1.1 REST client, an HTTP/3 QUIC stream, a WebSocket JSON frame, or an internal in-process test.

## 2. The Five Architectural Pillars

```
                     ┌───────────────────────────────────┐
                     │          Unified Port             │
                     │  (H1 / H2 / H3 / WS / RPC / gRPC) │
                     └─────────────────┬─────────────────┘
                                       │
                ┌──────────────────────┴──────────────────────┐
                ▼                                             ▼
     ┌──────────────────────┐                      ┌──────────────────────┐
     │  SIMD Hardware Scan  │                      │ Zero-GC Bump Arena   │
     │  (AVX2 / ARM NEON)   │                      │ (req.Arena() Scope)  │
     └──────────┬───────────┘                      └──────────┬───────────┘
                │                                             │
                └──────────────────────┬──────────────────────┘
                                       ▼
                     ┌───────────────────────────────────┐
                     │    Boot-Time Compiled Binder      │
                     │    (Zero-Reflection Fast Path)    │
                     └─────────────────┬─────────────────┘
                                       ▼
                     ┌───────────────────────────────────┐
                     │      Pure Domain Function         │
                     │  func(ctx, req) (*Res, error)     │
                     └───────────────────────────────────┘
```

### I. Pure Function Handlers & Declarative Ingestion
Handlers accept strongly-typed structs and return typed results and standard Go `error`. `sein` handles everything else:
* **Multi-Source Ingestion**: Single DTO fields automatically populate from `ctx`, `header`, `query`, `path`, `cookie`, `form`, and `json`.
* **Tag-Based Sanitization & Validation (`validate:"..."`, `sanitize:"..."`)**: `sanitize:"trim,lower,upper,single_space,digits_only"`, `validate:"uuid,email,url,enum=USD|EUR|JPY,min=1,max=1000"`.
* **Zero Reflection in Runtime**: Struct descriptors are analyzed and compiled into direct pointer arithmetic (`uintptr` memory offsets) at server boot time.

### II. Zero-GC Request Bump Arena (`req.Arena()`)
Traditional servers flood the Go runtime heap with short-lived objects (DTOs, parsed query maps, parameter slices, temporary strings), inducing stop-the-world GC pauses.
* Each request acquires a private, thread-safe lexical arena (`borrow.Scope`).
* Allocating bytes or strings takes **1 CPU cycle** (`offset += size`).
* Upon request completion, the arena bump pointer resets to 0. **Garbage collection overhead is literally zero.**

### III. SIMD Hardware Acceleration (AVX2 / ARM NEON)
`sein` bypasses naive byte-by-byte loops:
* Header block boundaries (`\r\n\r\n`), colon delimiters (`:`), and request lines are identified using vectorized 256-bit AVX2 / 128-bit NEON instructions (`simd.IndexCRLFCRLFVector`).
* Line parsing achieves **15–25 GB/s throughput per CPU core**.

### IV. Unified Multi-Protocol Reactor (Single Port)
No more spinning up disparate servers on different ports:
* **HTTP/1.1, HTTP/2, HTTP/3 (QUIC)**: Native zero-`net/http` protocol engines.
* **WebSockets (`ws.Handle`)**: Thread-safe connection hub with `PrecompileJSON` for $O(1)$ zero-copy broadcasting across 100,000+ active clients.
* **Server-Sent Events (`sse.Handle`)**: Declarative stream channels with built-in DTO parameter binding.
* **Strongly-Typed JSON-RPC (`rpc.Mount`)**: One-line reflective mounting of Go service structs into `POST /rpc/ServiceName.MethodName` APIs.
* **Zero-Dependency gRPC (`grpc.Mount`)**: High-performance HTTP/2 gRPC engine without Google's protobuf runtime conflicts.
* **WebTransport & MASQUE (`x/webtransport`, `tunnel/masque`)**: RFC 9297 datagrams and IP/UDP tunnel multiplexing.

### V. Declarative Matrix Routing & Domain Error Mapping
* **VersionMatrix (`r.Versioned("2", "3").Since("3")`)**: Clean API lifecycle management without routing bloatware.
* **Domain Error Mapping (`r.MapErrors(...)`)**: Domain errors (e.g. `ErrUserNotFound`) automatically serialize into standard RFC 9457 Problem Details HTTP responses with appropriate status codes (404, 409, 422).

## 3. The Grand Duality: aoni & sein

The ecosystem is built upon an aesthetic and architectural symmetry:

```
          ao [ ni ]              ◄──────────────►             se [ in ]
        (Outbound / To)                                     (Inbound / Into)
```

| Dimension | aoni (The Shadow / Chaos) | sein (The Citadel / Order) |
| :--- | :--- | :--- |
| **Role** | Outbound Client & Stealth Infiltrator | Inbound Server & High-Throughput Reactor |
| **Domain** | The hostile, wild Internet (DPI, WAF, Captchas) | The pristine, controlled compute cluster |
| **Pillars** | uTLS, Anti-DPI, p0f spoofing, Happy Eyeballs v3 | Zero-GC Bump Arena, SIMD Parsing, Pure Handlers |
| **Etymology** | *Ao-Oni* (青鬼 — Azure Demon / Evasive Intelligence) | *Sein* (German: Being / Essence; Japanese: 聖/静/正) |

Together, they establish a closed-loop distributed ecosystem where outgoing requests traverse the world with undetectable stealth (`aoni`), and incoming requests are processed with absolute zero-allocation mathematical precision (`sein`).

## 4. The "v1" Forever-Frozen Core Manifesto

1. **Backwards Compatibility as a Contract**:
   Code written for `sein v1.0.0` is guaranteed to compile and run unchanged on any `v1.x` version 5, 10, and 20 years from now.
2. **Immutable RFC Foundation**:
   The core is permanently locked to standardized IETF RFC and W3C specifications.
3. **Isolated Experimentation (`sein/x/...` & `sein/tunnel/...`)**:
   All cutting-edge extensions, experimental transports, and third-party adapters reside strictly in separate namespaces, protecting the core runtime from volatility.
