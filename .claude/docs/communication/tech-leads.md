# Comporta — for tech leads

## Scope
An LLM gateway in Go (stdlib `net/http`), OpenAI-compatible API, a single instance, SQLite. React dashboard embedded in the binary.

## Decisions and trade-offs
| Decision | Gain | Cost / risk |
|---|---|---|
| One OpenAI-compatible HTTP client instead of an interface with N providers | Less code; the fake exercises the same network path | A non-compatible provider would need an adapter |
| **Opt-in** exact cache | Never returns a "frozen" answer from a reasoning model | Fewer hits by default |
| Retry and fallback **only before the first byte** | Never duplicates content in the stream | A failure mid-stream becomes an error for the client |
| In-memory state (cache, rate limit, budget) | Simple and fast | With several instances it's no longer global: the trigger for Redis |
| Usage written in batches, dropped if the channel fills up | The API never waits for the database | Records can be lost under extreme peaks (counted in a metric) |
| Pattern masking + check digits | Few false positives, deterministic (preserves the prefix cache) | Doesn't catch names or addresses |
| Budget checked before the call, cost added after | No reservation logic on the hot path | Soft cap: concurrent requests can go slightly over |

## Risks
- DeepSeek prices and model names change: they're in the config, but need to be checked before real use.
- Singleflight with streams: followers get the whole response at once, not live (fan-out is in the backlog).
- Scaling horizontally requires Redis for the rate limit, the budget and the cache.

## Result (10/09)
12 commits, one per feature, on the `feat/mvp` branch. `go test -race ./...` green, 2 fuzz tests, integration tests for
every error code, k6 in 3 scenarios (README). ~1.7 ms overhead at p95; chaos with 50% 503s → 98.4% success.

## Security (10/09–10/10)
Fixed and merged:
- IDs never reused.
- Limit of 32 concurrent requests per client.
- Breaker per client and destination.
- Fallback on provider 401/402/403.
- Config requires a price for every model.
- Revocable admin session; login limited per IP, consumed on entry; X-Forwarded-For only behind a trusted proxy.

## How to validate
`go test -race ./...`, `BenchmarkOverhead` and the three k6 scenarios in `carga/`. The numbers go into the README.
