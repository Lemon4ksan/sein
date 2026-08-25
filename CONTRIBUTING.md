# Contributing to sein

Thank you for considering a contribution! `sein` is a high-performance, single-port protocol server reactor engineered for absolute stability, memory safety, and 0 B/op throughput under carrier-grade network loads. We welcome bug fixes, performance optimizations, and protocol refinements that benefit the ecosystem.

## Ground Rules

- **No application-specific logic.** `sein` is a protocol server reactor library. PRs that embed narrow business logic or ad-hoc application states will be closed.
- **Stability & Memory Safety First.** Zero allocations (0 B/op) must not compromise memory safety. Every zero-copy path must adhere to `borrow.Scope` invariants and pass borrow check verification.
- **Backward compatibility.** Public API breaks require a major version bump and a compelling architectural rationale.

## Getting Started

```bash
git clone https://github.com/lemon4ksan/sein
cd sein
go mod download
make race   # run tests with race detector
make lint   # run golangci-lint
```

## Workflow

1. **Fork** the repository and create a feature branch from `main`.
2. Write your code following the existing style (check `.golangci.yml`).
3. Add or update unit and fuzz tests — ensure `make race` and `make lint` pass cleanly.
4. Open a Pull Request against `main` using the provided PR template.

## Commit Style

Use conventional commits:

```
feat: implement RFC 8441 extended CONNECT WebSocket demux
fix: throttle RST_STREAM bursts to mitigate rapid reset flood
perf: optimize flat Huffman LUT decode in h2 framing
docs: document borrow scope lifecycle on streaming contexts
```

## Tests & Benchmarks

| Command | Purpose |
|---------|---------|
| `make test` | Fast unit tests |
| `make race` | Tests with `-race` and 60s timeout |
| `make bench` | Silicon hardware microbenchmarks |
| `make fuzz` | Security fuzzing across wire parsers |
| `make cover` | Coverage report |

New server features **must** include tests. Bug fixes **should** include a regression test.

## Code Style

- Run `gofmt` before committing (`golangci-lint` enforces this).
- Keep exported identifiers documented with Godoc comments.
- Avoid reflection (`reflect`) on request hot paths.
- Respect 64-byte CPU cache line padding for shared atomic structures.

## Questions

Open a [Discussion](https://github.com/lemon4ksan/sein/discussions) rather than an issue for general questions or architectural design proposals.
