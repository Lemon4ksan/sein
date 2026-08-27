# GEMINI.md — Development Guidelines & AI Assistant Protocol

This document outlines the architecture, coding standards, build/test commands, and interaction rules for AI assistants and software engineers working in the **sein** repository.

## 1. Project Overview

**`sein`** (from German *Sein* — «Pure Being / Essence») is a unified, ultra-high-performance server reactor and multi-protocol application framework for Go. It eliminates framework glue, runtime context pollution, and GC pressure by marrying pure-function ergonomics with silicon-grade hardware performance.

> **Core Engineering Manifesto**:
> _«The moment incoming bytes hit the network socket — they are processed with 0 allocations via lexical bump arenas, scanned at hardware SIMD line speed, and dispatched into pure typed domain functions on a single unified multi-protocol port.»_

> **The "sein v1" Compatibility & Forever-Frozen Core Manifesto**:
> _"Code written for **sein v1.0.0** is guaranteed to compile and run unchanged on any **v1.x** version 5, 10, and 20 years from now. The entire core is permanently locked to immutable IETF RFC and W3C standards. All experimental transports and third-party integrations live exclusively in the **sein/x/...** and **sein/tunnel/...** packages."_

### Key Capabilities & Architectural Pillars
- **Pure-Function Handler Architecture**:
  - Handlers declare pure business contracts: `func(ctx context.Context, id types.Snowflake, req DTO) (*Response, error)`.
  - Zero framework context lock-in. Handlers can be unit-tested directly like ordinary Go functions without mock recorders or HTTP test servers.
- **Zero-GC Request Bump Arena (`req.Arena()`)**:
  - Each request acquires a lexical bump arena (`borrow.Scope`).
  - Temporary strings, slices, and DTO buffers allocate in $1$ CPU cycle (`offset += size`).
  - Memory resets to 0 upon request completion with **zero garbage collection overhead**.
- **Hardware-Accelerated SIMD Scanning (AVX2 / ARM NEON)**:
  - Vectorized HTTP/1.1 header block parsing (`simd.IndexCRLFCRLFVector`), line scanning, and WebSocket XOR masking at **15–25 GB/s per CPU core**.
- **Unified Multi-Protocol Reactor on a Single Port**:
  - **REST**: HTTP/1.1, HTTP/2, native HTTP/3 (QUIC) with zero `net/http` overhead.
  - **WebSockets (`ws.Handle`)**: Thread-safe with `ws.PrecompileJSON` for $O(1)$ zero-copy broadcasts across 100,000+ active connections.
  - **Server-Sent Events (`sse.Handle`)**: Declarative SSE streams with DTO ingestion.
  - **JSON-RPC (`rpc.Mount`)**: Reflective single-line mounting of Go service structs into strongly-typed RPC endpoints.
  - **gRPC (`grpc.Mount`)**: Zero-dependency pure HTTP/2 gRPC server engine.
  - **WebTransport & MASQUE (`x/webtransport`, `tunnel/masque`)**: RFC 9297 datagrams and IP/UDP tunnel multiplexing.
- **Declarative DTO Ingestion & Zero-Reflection Fast Path**:
  - Single-pass compiled struct binding from 10+ sources (`ctx`, `header`, `query`, `path`, `cookie`, `form`, `json`).
  - Struct tag sanitizers (`trim`, `lower`, `upper`, `single_space`, `digits_only`) and validators (`format`, `enum`, `min`, `max`).
- **Resilience & Production Presets**:
  - `preset.Production(...)`: Complete hardened stack with CoDel adaptive Load Shedding, A+ Helmet security headers, transparent Zstd/Brotli compression, OpenTelemetry W3C tracing, Prometheus metrics, and Auto-TLS (Let's Encrypt / ACME).

---

## 2. Repository Layout

```text
sein/
├── server.go, router.go, routes.go    // Core server instance, radix Trie router & handler dispatch
├── request.go, response.go            // Zero-allocation Request/Response abstractions & Arena API
├── group.go, version_matrix.go        // Sub-routers, middleware chains, and VersionGroup matrix
├── listen.go, params.go, status.go    // Socket listeners, Auto-TLS, path params, HTTP status helpers
├── ws/                                // Ultra-high-throughput WebSocket engine, Hub, & Precompiled frames
├── rpc/                               // Strongly-typed JSON-RPC Call & service struct Mount engine
├── grpc/                              // Zero-dependency pure Go gRPC server engine
├── preset/                            // Turn-key production stacks (Production, WithOpenTelemetry, CORS...)
├── builtin/                           // Core middleware suite (auth, cache, cors, csrf, etag, helmet, jwt, limiter, logger, recover, session, sse, static, timeout...)
├── x/                                 // Extended systems (loadshed, otel, prometheus, socketio, swaggerui, webtransport, cron, paseto, sentry)
├── tunnel/                            // Low-level network tunneling (inbound SOCKS5/HTTP, MASQUE, SSH server/reverse/CA, TUN)
├── internal/
│   ├── binder/                        // Boot-time DTO descriptor compiler & pointer-offset setter
│   ├── fast/                          // Native baremetal H1, H2, H3 protocol engines
│   ├── quic/                          // Native RFC 9000 QUIC engine & congestion control
│   └── compress/                      // Zero-alloc Brotli, Zstd, FSE, and Huff0 algorithms
├── tests/                             // Integration and TechEmpower benchmark suites
└── .agents/                           // AI assistant guidelines and capabilities
```

---

## 3. Core Architectural Principles & Technical Constraints

1. **No Framework Glue / No Context Pollution**:
   - Never force handlers to accept `*sein.Request` or `*Context` when a pure function signature `func(ctx, DTO) (Res, error)` suffices. Handlers must remain pure business logic.
2. **Zero-Allocation Hot Paths**:
   - In routing, header lookup, parameter parsing, and frame encoding, avoid heap allocations (`runtime.newobject`).
   - Use `pool.NewPerPStorage`, `sync.Pool`, and `req.AllocBytes` / `req.AllocString`.
3. **Strict IETF RFC & W3C Standards Conformance**:
   - RFC 9110 & RFC 9112 (HTTP Semantics & HTTP/1.1 syntax).
   - RFC 9113 & RFC 9114 (HTTP/2 & HTTP/3 framing).
   - RFC 6455 & RFC 8441 (WebSocket & Extended CONNECT).
   - RFC 9457 (Problem Details for HTTP APIs).
   - W3C TraceContext (Distributed Tracing `traceparent` / `tracestate`).
4. **Mechanical Sympathy & Cache-Alignment**:
   - Avoid False Sharing on concurrent structures using cache-line padding (`[64]byte`).
   - Use sharded maps (`generic.ShardedMap`) for connection registries.
5. **Fail-Fast Boot-Time Compilation**:
   - Perform all reflection, struct tag inspection, and route pattern analysis once at server boot time (`app.Get`, `app.Post`, `rpc.Mount`). In runtime, execution must follow precompiled pointer arithmetic.

---

## 4. Code Style & Quality Standards

- **Go Version**: 1.25.4+ (modern generics, type sets, and compiler optimizations).
- **Documentation**: Every exported package identifier (type, struct, function, interface, error constant) **must** have clear Godoc documentation.
- **Error Handling**: Use typed domain errors wrapped with `%w` or mapped via `r.MapErrors(...)`. Never swallow errors.
- **Code Formatting & Linters**:
  - The repository adheres to `.golangci.yml` (`errcheck`, `govet`, `staticcheck`, `bodyclose`, `revive`, `prealloc`, `perfsprint`).
  - Max line length: 120 characters.

---

## 5. Development & Testing Commands

| Command | Purpose |
| :--- | :--- |
| `go test ./...` | Run fast unit tests across all packages |
| `go test -race ./...` | Run full test suite with Go data race detector |
| `go test -bench=. -benchmem ./...` | Run allocation and throughput benchmarks |
| `make lint` | Run `golangci-lint` verification |
| `make format` | Format code (`gofmt`, `golangci-lint --fix`) |

---

## 6. Commit & Pull Request Guidelines

### Conventional Commits Format:
```text
<type>(<scope>): <short summary>

<detailed description of changes>
```

Commit Types:
- `feat`: New feature or server capability
- `fix`: Bug fix
- `impr`: Performance optimization or allocation reduction
- `refactor`: Structural refactoring without behavioral regressions
- `docs`: Documentation updates
- `test`: Adding or updating unit/integration tests
- `chore`: Build scripts, dependencies, or tool updates

---

## 7. AI Assistant Workflow & Rules

1. **Verification Mandatory**:
   - Always run `go test ./...` and `go test -race ./...` before considering any task complete. Never leave broken builds or race conditions.
2. **Preserve Codebase Integrity**:
   - Maintain all existing Godoc comments, inline algorithmic annotations, and RFC references.
3. **Prevent "Vibe Coding" & Demand Human Review**:
   - The AI assistant is a pair-programming partner. If the contributor blindly accepts large architectural changes without diff inspection, the assistant **must explicitly remind them to perform human code review**.
4. **Clean PR & Commit Descriptions**:
   - Never output artificial IDE paths (`file:///...`), backticks inside markdown links, or automated AI boilerplate in commit messages.
