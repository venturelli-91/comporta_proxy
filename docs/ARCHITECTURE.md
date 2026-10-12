# Comporta — Reference architecture

> The project's decision record. Research done on 2026-10-09. CLAUDE.md points here:
> when a decision changes, update this file together with the code.

## 1. Summary of decisions

| Topic | Decision | Why |
|---|---|---|
| HTTP server | standard library `net/http` | native SSE streaming, HTTP/2, `context` and `httptest`; the bottleneck is the provider (seconds), not the HTTP parser (microseconds) |
| Provider client | one `http.Client` per provider with a tuned `Transport` (high `MaxIdleConnsPerHost`) | the default is **2** idle connections per host, which destroys connection reuse under load |
| API format | OpenAI-compatible (`/v1/chat/completions`) | adoption by changing only the base URL |
| Request body | the fields the gateway uses + `json.RawMessage` for the rest | the gateway changes a few fields without losing the ones it doesn't know |
| Streaming | line-by-line relay with flush per event | first-token latency preserved; PII restoration and `usage` capture along the way |
| Retry and fallback | only **before the first byte** is sent to the client | once the stream has started there is no going back |
| Provider protection | per-model concurrency semaphore + per-client limit + circuit breaker (`sony/gobreaker/v2`) | DeepSeek limits concurrency per account (section 3) |
| Rate limit | MVP: **requests/min** per client (token bucket, `x/time/rate`). Next step: estimated tokens reconciled with `usage` | LLM requests range from 10 to 100k tokens; the MVP limits volume, the budget (402) limits spend |
| Gateway cache | exact, **opt-in**, in-memory LRU + TTL + `singleflight` | models with reasoning on by default are not deterministic |
| Provider cache | keep prefixes stable and **measure** `prompt_cache_hit_tokens` | DeepSeek's prefix cache is automatic and makes input much cheaper |
| Usage logging | buffered channel + batched writes (SQLite WAL); drop and count when full | no database write on the critical path |
| Hot configuration | immutable snapshot in an `atomic.Pointer`, swapped when the admin changes something | lock-free read on every request |
| Front end | React + Vite + TS, TanStack Query, Recharts; served by Go itself with `//go:embed` | a single binary to deploy |
| Streaming playground | `fetch` + `ReadableStream` (not `EventSource`) | `EventSource` only does GET and can't send an authorization header |
| Deploy | distroless container; Cloud Run (GCP) | same environment as the job |

### State as of 2026-10-09 (branch `feat/mvp`)
Phases 1 and 2 and the shadow mode from phase 3 are implemented. Differences from the plan above:
- **Providers:** a single OpenAI-compatible HTTP client (`internal/provedor`), no `Provider` interface. The fake is an `http.Handler` speaking the same protocol.
- **Semaphore:** per alias (`concorrencia` in `comporta.json`), released only when the response body is closed (end of the stream).
- **Breaker:** opens with ≥ 5 calls and ≥ 50% failures; the retry lives inside it; client 4xx counts as success; cancellation is excluded.
- **Singleflight:** the leader reads the whole response and everyone gets a replay (no live fan-out).
- **Shadow:** only on the uncached path; non-streaming call, its own context, comparison in `/admin/uso`. On shutdown (8 s deadline, below Cloud Run's 10 s) stuck shadows are cancelled.
- **No `Repository`, no `uso_minuto`:** the report scans the raw table (debt recorded in `.claude/docs/backlog.md`).
- **Measured numbers:** README and `.claude/docs/features/11-load.md`.

## 2. Lessons from existing gateways

- **LiteLLM** (Python) is the most used; **Bifrost** (Go, Apache 2.0) and **Helicone** (Rust) were born focused on performance; **Portkey** (TypeScript) and **Kong/Envoy AI Gateway** come from the API gateway world.
- Bifrost separates the **core** (`core/`: providers and schemas), **transport** (`transports/bifrost-http`), **storage** (`framework/`: config, logs, vectors) and **plugins** (governance with virtual keys and budgets, semantic cache, telemetry). It's a good organizational reference: Comporta follows the same idea with `internal/` packages.
- Bifrost publishes an **overhead of ~1 ms per request** versus ~40 ms for LiteLLM, measured against a 60 ms fake provider. Those are the vendor's own numbers and the article repeating them is biased, but the **engineering lessons** it cites match what is known about Go:
  1. log **asynchronously and in batches**;
  2. **reduce allocations** per request (buffer pools);
  3. **tune the connection pool**: with the default transport, reuse dropped from ~90% to ~40% under load, adding 50–100 ms per request.
- **Comporta's goal:** measure and publish its own overhead (p50/p99) against the fake provider. An honest, reproducible number beats a big one.

## 3. What the DeepSeek API imposes on the project

Source: official documentation (api-docs.deepseek.com), consulted on 2026-10-09. **Prices and names change: all of this goes in configuration, never in code.**

| Fact | Consequence for the gateway |
|---|---|
| Current models: `deepseek-flash` (V4.1-Flash) and `deepseek-v4-pro`; 1M context | our own aliases (`rapido`, `forte`) mapped to models in the config |
| Reasoning mode (`thinking`) **on by default**; `temperature` has no effect in this mode; the response includes `reasoning_content` | the cache can't rely on `temperature: 0`; caching is opt-in. Record `reasoning_tokens` in the cost |
| **Automatic prefix cache**: `usage.prompt_cache_hit_tokens` and `prompt_cache_miss_tokens`; a hit requires an identical prefix | don't reorder messages; **deterministic** PII masking (same text → same result); show the savings in the dashboard |
| Prices per 1M tokens with **much cheaper cached input** and **peak hours** (off-peak costs half; peak: 01–04h and 06–10h UTC on weekdays) | the cost engine knows the time; an "off-peak savings" metric; a queue of deferrable jobs (phase 3) |
| **Concurrency limit per account**: 2,500 (`deepseek-flash`) and 500 (`deepseek-v4-pro`); above that, 429 | per-model semaphore slightly below the limit + per-client limit, so one client can't take everything |
| `user_id` field (`[a-zA-Z0-9\-_]+`, no personal data) for per-user limits | send a hash of the client ID, never the name |
| While waiting, the server sends **empty lines** (non-streaming) or **SSE comments** `: keep-alive` (streaming); it closes if inference doesn't start within **10 min** | the parser ignores keep-alives; the gateway has **its own** time-to-first-token timeout (e.g. 30 s) and doesn't count keep-alives as progress |
| `stream_options: {include_usage: true}` makes the last chunk carry `usage`; the stream ends with `data: [DONE]` | the gateway injects `include_usage` to measure the cost of every stream |
| Errors: 400, 401, 402 (balance), 422, **429**, **500**, **503** | retry only on 429/500/503/network, honoring `Retry-After`; 401/402 become an alert for the admin, not a retry |

## 4. Life of a request

```
client ──▶ [1] recover + request ID
           [2] authentication: hash(ck_...) → client (in-memory snapshot)
           [3] body limit + decode (known fields + RawMessage)
           [4] admission: rate limit + budget                         ──✗ 429 / 402
           [5] LGPD masking (if enabled for the client)
           [6] exact cache (opt-in) ── hit ──▶ respond (SSE replay if streaming)
                   │ miss
           [7] singleflight per cache key (cacheable requests only)
           [8] router: alias → ordered list [primary, fallback...]
           [9] per destination: model semaphore → circuit breaker → call
                   └ retry with backoff+jitter BEFORE the first byte; otherwise the next destination
          [10] relay: reads the provider's SSE, ignores keep-alives, restores PII,
               captures usage from the last chunk, writes and flushes to the client
          [11] usage event → buffered channel → batch writer
```

Order matters: authentication and admission come **before** any expensive work; masking comes **before** the cache (the cache never sees personal data); the retry sits **inside** the breaker (so the breaker counts the final attempt).

## 5. Hot-path performance

- **Transport:** clone `http.DefaultTransport` and tune `MaxIdleConns`, `MaxIdleConnsPerHost` and `MaxConnsPerHost` (100–1000, depending on the provider's limit), `IdleConnTimeout` 90 s, `ForceAttemptHTTP2: true`. **No** global `Client.Timeout`, because it kills long streams; deadlines come from the `context` and the per-stage timeouts (section 7).
- **JSON:** decode only what's needed. In the stream, don't fully `Unmarshal` every chunk: only the chunks that need PII restoration and the last one (with `usage`).
- **Configuration:** `atomic.Pointer[snapshot]`, no mutex on reads.
- **Async usage:** a buffered `chan Evento` (e.g. 10k). If it fills up, **drop and increment a metric**: losing a usage record is acceptable, blocking the API is not.
- **Runtime:** `GOMEMLIMIT` below the container limit.
- **Measurement:** `BenchmarkOverhead` (gateway + zero-latency fake provider) with `-benchmem`, and k6 against a 60 ms fake provider to compare with the number Bifrost publishes.

## 6. Cache

**Two layers with different roles:**

1. **The gateway's exact cache** (whole response):
   - Opt-in: the `X-Comporta-Cache: ligado` header or the client's policy. With reasoning on, the same question produces different answers; reusing one has to be an explicit choice.
   - Key: `sha256(client + body with placeholders already applied)`. The scope is **per client** (so one client never sees another's response).
   - LRU with TTL and a memory limit.
   - Streamed responses are stored whole and returned as an **SSE replay**.
   - `singleflight` on the same key: N identical concurrent requests become 1 paid call. The leader reads the whole response; the others wait for it (live fan-out is the advanced version).
2. **DeepSeek's prefix cache** (automatic, input only):
   - The gateway **stays out of the way**: it doesn't reorder messages and masks PII deterministically.
   - The dashboard shows `prompt_cache_hit_tokens` and how much it saved.

**`X-Comporta-Enviado`** (response header, LGPD clients only): the user's last message as it went out to the provider, already masked, base64-encoded and cut at 2 KB. It's a header so the body keeps the OpenAI format SDKs expect; the Playground uses it to show the before and after.

**Semantic cache** (similar questions): later. It needs embeddings (the DeepSeek documentation I consulted only covers chat, so it would take another provider or a local model) and carries a real risk of returning a wrong answer. If it's built, it needs a high threshold and measurement of false hits.

## 7. Resilience

| Mechanism | Initial setting |
|---|---|
| Connection timeout | 5 s |
| Time-to-first-token timeout | 30 s (keep-alives don't count) |
| Between-chunks timeout | 60 s without useful data |
| Total deadline | 10 min for non-streaming calls; streams are bounded by the watchdog and by the client's `context` (disconnect → cancel upstream) |
| Retry | up to 3 attempts, only on 429/5xx/network errors, **only before the first byte**, exponential backoff with jitter, `Retry-After` honored, within the deadline |
| Circuit breaker | `gobreaker/v2` per client and destination (provider+model); opens on a high failure rate within a window; `IsSuccessful` treats client 4xx (400/413/422) as success because they don't mean the provider is sick |
| Fallback | ordered list per alias; breaker open → next in the list |
| Concurrency | per-model semaphore below the account limit + max concurrent requests per client; a short wait, then 429 + `Retry-After` |

Error mapping for the client: 401 (invalid key), 402 (budget exhausted), 429 (client or provider limit), 502 (invalid provider response), 503 (breaker open / no destination available), 504 (timeout). Always `{"erro": {"codigo", "mensagem"}}` (`error`, `code`, `message`), without leaking internal details.

## 8. Rate limit, budget and cost

- **Current state:** rate limit in requests/minute (token bucket) and the budget checked **before** the call (`month's spend >= limit` → 402), with the real cost added **after** it, from `usage`. It's a soft cap: with up to 32 concurrent requests per client, spend can exceed the limit by the cost of those in-flight calls.
- **Hard cap (later):** pre-reserve the maximum cost (estimated input + `max_tokens` of output) and settle the difference with the real `usage`; with several instances, the reservation becomes an atomic operation in Redis.
- **Shadow outside the budget:** a shadow call is a cost for whoever runs the gateway, not for the client. It's recorded (with its cost) only in the comparison, and the spend is bounded by the 16 concurrent shadow slots.
- **Cost:** `(miss × miss_price + hit × hit_price + output × output_price)` using the table for the time of day (peak/off-peak), all from configuration. Stored in micro-dollars (`int64`), never `float` for money.
- **Savings metrics** in the dashboard: the gateway cache, DeepSeek's prefix cache and off-peak hours.

## 9. LGPD

See the rules in CLAUDE.md. Architecture points:

- **Deterministic per-request** masking: placeholders numbered by order of appearance. Same text → same masked text, which preserves both caches.
- Check-digit validation for CPF/CNPJ and Luhn for cards (fewer false positives).
- Stream restoration with a small buffer for placeholders split across chunks.
- The placeholder → real value map exists only in the request's memory.
- **Quality is measured, not assumed:** `TestQualidadeDeteccao` scores precision and recall per type on a labeled corpus (`internal/lgpd/testdata/corpus.txt`) with a floor at the measured values. The detector isn't tuned to the corpus: a fix for a miss comes with new lines it wasn't written against.
- `messages[].content` is masked as a string or as an array of text parts. **Fail closed:** with LGPD on, a non-text part (image, audio) gets a 400 instead of going out unmasked; `null` content (assistant tool calls) is left as is.
- **Out of reach today:** `tools`, `tool_calls[].function.arguments`, `name` and `user` go out unmasked, and in streams only `content` and `reasoning_content` are restored.

## 10. Data

SQLite (`modernc.org/sqlite`, no cgo) in **WAL** mode. No repository interface yet: it will be born in the consumer when Postgres arrives.

| Table | Contents |
|---|---|
| `clientes` (clients) | id, name, limits, monthly budget, cache and LGPD policy, allowed models |
| `chaves` (keys) | id, client_id, visible prefix (`ck_ab12…`), SHA-256 hash, created_at, revoked_at |
| `uso` (usage) | client, model, destination, tokens (hit, miss, output, reasoning), cost in micro-dollars, total latency, time to first token, cache, status, timestamp. **No message content** |

Batched writes (every 500 events or 1 s). A per-minute aggregate table (`uso_minuto`) comes in when the raw scan shows up in a profile.

## 11. Observability

- **Prometheus** (`/metrics`): requests, errors, cache hits, masked PII, dropped shadows and usage events, cache bytes, breaker state per destination. Next: latency and labels per client/model (low cardinality: client IDs yes, request IDs never) and semaphore occupancy.
- **slog** in JSON with `request_id`.
- OpenTelemetry in phase 3.

## 12. Front end

- **Vite + React + TypeScript**, **TanStack Query** (server data cache and refresh every few seconds), **Recharts** for charts, Tailwind.
- Screens: overview (spend, savings, p95, error rate), clients and keys (create, revoke, show the key only once), shadow comparison, provider state (breaker), playground.
- **Playground:** `fetch` with `ReadableStream` to read the SSE of an authenticated POST; shows what the provider received next to the response, time to first token and the cost of the call.
- **Admin authentication:** sign in with the `ADMIN_TOKEN` → a random session cookie `HttpOnly; Secure; SameSite=Strict`. The token never goes into `localStorage`. Login limited to 5 attempts per IP (consumed on entry); the IP comes from the last `X-Forwarded-For` entry only with `K_SERVICE` or `TRUST_PROXY=1`, otherwise from the connection address.
- **Build:** `web/dist` embedded in the Go binary with `//go:embed`; Go serves the files and falls back to `index.html` for the React routes (unknown `/v1/*` and `/admin/*` routes return a JSON 404). `index.html` goes out with `Cache-Control: no-cache` (it's the only unhashed file and points at the hashed bundle), so a deploy is picked up on the next visit.

## 13. Tests

| Type | What |
|---|---|
| Unit (table-driven) | cost with peak/off-peak, config validation and defaults, masking and restoring, retry, cache |
| Concurrency (`-race`) | singleflight: N identical requests → 1 call; cache and snapshot under concurrent writes |
| Real provider against `httptest.NewServer` | mimics DeepSeek: stream with `: keep-alive`, `usage` in the last chunk, `[DONE]`, 429 with `Retry-After`, 503 |
| Fuzz | mask→restore returns the original, including split stream chunks |
| Benchmark | gateway overhead with `-benchmem` |
| Load (k6) | constant load, cache, and **chaos**: fake provider with 50% 503s to show the breaker and fallback working |

The **fake provider** is a centerpiece: latency, time per token and error rate are configurable.

## 14. Deploy (GCP)

- Distroless nonroot image with the single binary (API + dashboard).
- **Cloud Run**: tune per-instance concurrency, a request timeout that fits long streams, and minimum instances to avoid cold starts; secrets in **Secret Manager**.
- With more than one instance, the in-memory rate limit and cache stop being global: that's the trigger for Redis (Memorystore) in phase 3.
- The public demo runs on Render instead (free tier without a card), with the fake provider only.

## 15. Phases

**Phase 1 — by 10/13 (interview):** fake provider + DeepSeek; `/v1/chat/completions` with and without streaming; per-client keys; rate limit; per-model semaphore; retry + breaker; opt-in exact cache + singleflight; batched usage; dashboard with overview, clients and playground; k6 with numbers in the README.

**Phase 2:** LGPD masking (non-streaming, then streaming); budget (402); fallback between models; token estimation with reconciliation; Prometheus metrics.

**Phase 3:** shadow mode with a comparison report; a queue of deferrable jobs to run off-peak; Redis for shared state; Postgres; OpenTelemetry; Cloud Run.

## 16. Sources

- DeepSeek official documentation: Chat Completion, Context Caching, Models & Pricing, Rate Limit and Error Codes (api-docs.deepseek.com).
- Bifrost: the github.com/maximhq/bifrost repository and the benchmarks published at getmaxim.ai/bifrost (vendor numbers).
- dev.to, "We benchmarked 5 LLM gateways at 5000 RPS" (lessons on async logging, allocations and connection pools; a text biased toward Bifrost).
- MLflow, "Gateway LLM rate limits" (token-based limits, reconciliation, handling 429).
- tianpan.co, "Multi-tenant LLM noisy neighbor isolation" (admission, priorities, load shedding).
- LoginRadius, "Tune the Go HTTP client for high performance" (`http.Transport` defaults).
- pkg.go.dev/github.com/sony/gobreaker/v2.
