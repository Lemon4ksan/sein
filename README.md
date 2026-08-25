<div align="center">

# sein

### The Sane Single-Port Protocol Server Reactor for Go

[![License](https://img.shields.io/github/license/lemon4ksan/sein?style=flat-square)](LICENSE)
[![Status](https://img.shields.io/badge/status-active%20development-blue?style=flat-square)](#)
[![Ecosystem](https://img.shields.io/badge/ecosystem-foundation-blueviolet?style=flat-square)](https://github.com/lemon4ksan/foundation)

> _"In backends, madness is the default. Let **sein** be your light of sanity."_

#### English • [Русский](README_RU.md) • [Conceptual Manifest](docs/CONCEPT.md)

</div>

## 1. Project Vision (The One-Sentence Pitch)

**`sein`** is an in-development sovereign Go server reactor engineered for zero-allocation execution (**0 B/op**), designed to unify **HTTP/1.1, HTTP/2, HTTP/3 (QUIC), WebSockets, and gRPC on a single port `:443`** without reverse proxies, with mathematically verified memory safety (`borrow.Scope`) and hardware-level resistance to network DoS attacks.

## 2. The Problem: Ending the Server Trilemma

Modern Go backend architectures are torn between three compromises:

1. **`net/http` / `Gin`:** Safe and standard, but incurs multiple allocations per request, GC overhead, and lock contention on multi-core hardware.
2. **`fasthttp` / `Fiber`:** Fast on HTTP/1.1, but lacks native HTTP/2 and HTTP/3 support, while manual buffer pooling risks Use-After-Free corruption when references escape into goroutines.
3. **`grpc-go` / `Nginx`:** Requires maintaining separate ports, allocating deep Protobuf pointer trees, and configuring external reverse proxies to tie protocols together.

**`sein` targets the synthesis:** high-throughput zero-copy execution, type safety, and full-stack IETF protocol unification on a single socket.

## 3. The Four Fundamental Invariants

### I. Single-Port Protocol Matrix (Port `:443` Unification)
* **Single Listening Endpoint:** Listen simultaneously on TCP `:443` and UDP `:443`.
* **Zero Reverse Proxy Layer:** Dispatches incoming connections directly at wire level without Nginx or Envoy sidecars.
* **Fast ALPN & Connection ID Demux:** Sub-microsecond protocol routing via TLS 1.3 ALPN (`h2`, `http/1.1`) and QUIC Connection IDs (`h3`).
* **Native Multiplexed WebSockets:** WebSockets connect over existing HTTP/2 and HTTP/3 streams via **RFC 8441** and **RFC 9220** (Extended `CONNECT`), eliminating TCP socket hijacking (`Hijack()`).

### II. Silicon Determinism & Memory Safety (Target 0 B/op)
* **Core-Pinned Allocation Rings:** Buffer and context recycling via `foundation/silicon/pool` (`PerPStorage`), removing cross-core synchronization locks.
* **Flat Huffman LUT & Vectorized SIMD:** Fast HPACK decoding using precomputed lookup matrices and AVX2/BMI2 delimiter scanning.
* **Compile-Time Borrow Safety:** Integration with `borrow.Scope` and `vortex check` ($P * Q$ Separation Logic) to ensure zero-copy slices cannot escape into background goroutines without explicit cloning.

### III. Adversarial Immunity by Design (Anti-DoS Defense)
* **Anti-Rapid Reset Defense (CVE-2023-44487):** Per-socket token buckets to throttle `RST_STREAM` frame floods.
* **Anti-Slowloris Dynamic Flow Control:** Tracking physical transfer speed (`MinTransferRate`) rather than relying solely on static wall-clock timeouts.
* **Fair Queuing Stream Scheduler:** Dynamic interleaving of `DATA` frames across active multiplexed streams to prevent head-of-line blocking.
* **Compression Bomb Armor:** Bounded decompression state limits for HPACK and QPACK to prevent memory exhaustion attacks.

### IV. Declarative Symbiosis with `vortex` (No-Glue Architecture)
* **Single AST Contract:** Services and schemas defined via Go interfaces, OpenAPI 3.1, or Protobuf.
* **Reflection-Free Code Generation:** `vortex gen` compiles interfaces directly into Radix Trie routers (`foundation/silicon/trie`) and typed zero-copy DTO binders.

## 4. Memory Safety Model: Compile-Time Verification

To eliminate Use-After-Free bugs without sacrificing zero-allocation performance, `sein` relies on static verification:

```go
// ❌ Static check failure (B001 - Scoped Borrow Escape):
srv.POST("/api/v1/events", func(c *sein.Context) error {
    data := c.Body() // Borrowed slice tied to request lifetime
    go func() {
        processEvent(data) // vortex check: borrowed memory escapes to un-synchronized goroutine
    }()
    return c.SendStatus(200)
})

// ✅ Verified pattern (Explicit ownership clone for async handoff):
srv.POST("/api/v1/events", func(c *sein.Context) error {
    data := c.BodyClone() // Explicit allocation when asynchronous lifetime is needed
    go func() {
        processEvent(data)
    }()
    return c.SendStatus(200)
})
```

* **Escape Prevention (`B001`):** Verifies that borrowed byte buffers never escape past request lifecycles.
* **Disjoint Intervals (`B003`):** Proves non-overlapping slice mutations during zero-copy parsing.
* **Linear Lifecycle (`B011`):** Enforces strict state progression ($\text{Acquired} \to \text{Frozen} \to \text{Released}$).

## 5. Architectural Principles for High Throughput

`sein` is designed around microarchitectural hardware sympathy:

* **Eliminating GC Mark-Assist:** Keeping per-request working memory off the garbage-collected heap.
* **Cache Line Alignment:** Structs with shared atomic fields aligned to 64-byte L1/L2 cache lines (`cpu.CacheLinePad`) to prevent false sharing.
* **Lock-Free Core Sharding:** Per-CPU execution queues (`PerPStorage`) instead of centralized global mutexes.
* **Zero-Syscall Timestamps:** Monotonic clock readings via direct atomic reads (`silicon/clock`).

## 6. Target API Example

```go
package main

import (
	"log"

	"github.com/lemon4ksan/sein"
	"github.com/lemon4ksan/sein/option"
	"github.com/lemon4ksan/sein/ws"
)

type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

type UserResponse struct {
	ID       uint64 `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

func main() {
	srv := sein.NewServer(
		option.WithAddr(":443"),
		option.WithTLS("cert.pem", "key.pem"),
		option.WithHTTP3(true),            // Native HTTP/3 QUIC on UDP :443
		option.WithFairQueuing(true),      // Stream DATA frame balancing
		option.WithAntiRapidReset(1000),   // Max 1,000 RST_STREAM/sec per connection
		option.WithMinTransferRate(1024),  // Anti-Slowloris rate monitoring
	)

	// 1. REST API (Zero-Alloc JSON binding)
	srv.POST("/api/v1/users", func(c *sein.Context) error {
		var req CreateUserRequest
		if err := c.BindJSON(&req); err != nil {
			return c.SendStatus(400)
		}
		return c.SendJSON(201, UserResponse{
			ID:       c.NextID(),
			Username: req.Username,
			Email:    req.Email,
		})
	})

	// 2. WebSockets (Multiplexed stream over RFC 8441 H2 / RFC 9220 H3)
	srv.WS("/ws/feed", func(conn ws.Conn) {
		defer conn.Close()
		for {
			msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			_ = conn.WriteMessage(ws.OpText, msg)
		}
	})

	// 3. gRPC (Native 5-byte framing on the same port)
	srv.GRPC("/UserService/GetUser", func(c *sein.GRPCContext) error {
		return c.SendProto(200, &UserResponse{ID: 1, Username: "alice"})
	})

	log.Println("sein reactor starting on :443 (TCP+UDP)")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
```

## 7. Global Ecosystem

`sein` is part of a cohesive networking and runtime ecosystem:

* **`foundation`** — Hardware silicon substrate (SIMD vectors, off-heap slabs, lock-free rings, Per-P storage).
* **`aoni`** — Outbound client reactor (Chromium stealth, evasion, uTLS, Happy Eyeballs v3, MASQUE).
* **`sein`** — Inbound server reactor (Single-port `:443`, 0 B/op, anti-DoS armor, RFC 8441/9220 WebSockets).
* **`vortex`** — Declarative AST toolchain ($P * Q$ borrow checker, contract compiler, mock generation).
* **`porthack`** — High-concurrency load testing engine and benchmark validator.
* **`decon`** — Perimeter edge proxy and traffic cleaner.
* **`niko`** — Sovereign lightweight runtime orchestrator.

## 8. Repository Layout

```
sein/
├── option/       // Server configuration options (WithAddr, WithTLS, WithHTTP3, WithAntiRapidReset)
├── router/       // Zero-alloc Radix Trie router (foundation/silicon/trie)
├── h1/           // HTTP/1.1 pipelining and keep-alive reactor
├── h2/           // Native HTTP/2 frame multiplexer and HPACK flat LUT
├── quic/         // Sovereign Pure-Go RFC 9000 QUIC transport engine
├── h3/           // Native HTTP/3 RFC 9114 & QPACK RFC 9204 codec
├── ws/           // RFC 8441 & RFC 9220 WebSocket engine
├── grpc/         // Zero-copy 5-byte framing gRPC / gRPC-Web handlers
├── security/     // Anti-Rapid Reset token buckets, Anti-Slowloris rate guards
├── compress/     // Silicon-accelerated compression engines (flate, zstd, brotli, gzip)
├── context.go    // Zero-alloc request Context with borrow.Scope lifecycle
└── server.go     // Single-port listener and ALPN/CID demultiplexer
```

## 9. License

Licensed under the **BSD 3-Clause License**. See [LICENSE](LICENSE) for details.

<div align="center">
  <sub>In backends, madness is the default. Let <b>sein</b> be your light of sanity.</sub>
</div>
