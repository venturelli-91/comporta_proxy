# Comporta — MVP design (Phase 1 + the pitch's differentiators)

Date: 2026-10-09. Interview: 2026-10-14. Basis: `CLAUDE.md` and `docs/ARCHITECTURE.md`.
Historical document: it records the design as approved on 10/09 (originally written in Portuguese).

## 1. Understanding

**What was asked:** a public, measured LLM gateway in Go that backs the 30-second pitch in `CLAUDE.md` in an
interview for a Go proxy job on GCP. Complete documentation (spec, plan, backlog, checklist, changelog,
architecture). Simplicity checked with ponytail all along.

**Success criterion:** every sentence of the pitch has code, a test and a number behind it:

| Pitch sentence | Proof |
|---|---|
| masks CPF, email, phone and restores them on the way back | `internal/lgpd` with table-driven + fuzz tests; an end-to-end test with the fake provider echoing the masked text |
| each application has its own key, limit and budget | `ck_` keys hashed with SHA-256; 401, 429 with `Retry-After`, 402 |
| identical requests cached and deduplicated | concurrent test: N identical requests → 1 provider call |
| timeout, retry and circuit breaker | tests with the fake returning 503; fallback to the next destination |
| shadow mode | a second model called in the background; comparison in the dashboard |
| measured with k6 | `carga/carga.js` against the fake; numbers in the README |

**Assumptions (mine, not the user's):**
- DeepSeek prices in `comporta.json` are examples; the official table must be checked before real use.
- A single instance (cache, rate limit and budget in memory). Redis waits for phase 3.
- A single admin (`ADMIN_TOKEN`), no multi-user.

## 2. Approaches considered

1. **One OpenAI-compatible HTTP client + a fake provider speaking the same protocol (chosen).**
   DeepSeek is OpenAI-compatible, so "deepseek" and "openaicompat" are the same code with another
   `base_url`. The fake is an `http.Handler` that mimics DeepSeek (keep-alive, `usage` in the last chunk,
   `[DONE]`, random 503s). Tests and k6 go through the **same** network path as the real provider.
2. A `Provider` interface with three implementations (what the initial spec suggested). Three pieces of code
   to maintain, and an in-memory fake would skip exactly the network code that breaks most.
3. A generic reverse proxy (`httputil.ReverseProxy`). Good for blind forwarding, bad for masking,
   caching and measuring tokens in the middle of a stream.

## 3. Architecture

```
cmd/comporta/main.go       config from env, wires everything, starts the fake if there's no DEEPSEEK_API_KEY, graceful shutdown
internal/gateway/          config (comporta.json), cost (micro-dollars, peak), router (semaphore → breaker → retry → fallback)
internal/provedor/         OpenAI-compatible HTTP client (tuned transport) + Falso (fake, an http.Handler)
internal/resiliencia/      Retry with exponential backoff + jitter + Retry-After
internal/cache/            LRU with TTL and a byte limit (container/list)
internal/lgpd/             Masker and restorer (including in streams)
internal/armazenamento/    SQLite (modernc): clients, keys (hashed), usage; batch writer
internal/httpapi/          routes, middlewares, chat (with/without streaming), admin, metrics, embedded dashboard
web/                       React dashboard (Vite + TS + TanStack Query + Recharts + Tailwind)
carga/                     k6
```

### Path of `POST /v1/chat/completions`

1. `recover` + `X-Request-Id` + structured log (no content).
2. Authentication: `Bearer ck_...` → SHA-256 → client in the snapshot (`atomic.Pointer`, lock-free).
3. Admission: per-client token bucket (`x/time/rate`, req/min) → 429 + `Retry-After`;
   monthly budget in micro-dollars → 402; model outside the client's list → 403.
4. Limited body (1 MiB), decoded into `map[string]json.RawMessage` (unknown fields preserved).
   `model` is an alias from the config (`rapido`, `forte`).
5. LGPD (if enabled for the client): masks `messages[].content` (text). The map lives only in the request.
6. Cache (opt-in: the `X-Comporta-Cache: ligado` header or the client's policy): key
   `sha256(client + masked body)`. Hit → respond (JSON or SSE replay). Miss → `singleflight`
   on the same key; the leader fetches the whole response and stores it; the others receive the same bytes.
7. Router: alias → destinations `[primary, fallback...]`. Per destination: the model's semaphore (short
   wait, otherwise 429) → circuit breaker (`gobreaker/v2`) → retry (up to 2, only 429/5xx/network, before the
   first byte) → next destination if the provider failed.
8. Relay: without streaming, reads the whole JSON, restores PII and returns it. With streaming, reads the SSE line by line,
   ignores `: keep-alive`, restores PII with a buffer for split placeholders, captures `usage` from the last
   chunk, writes and flushes. Watchdog: 30 s until the first useful data, 60 s between data.
9. Usage: event → buffered `chan` (10k) → batch writer (500 events or 1 s). Channel full:
   drop and count in the metric. Shadow (if configured for the alias): a goroutine with its own context
   calls the shadow model without streaming and records a `shadow=1` event for comparison.

### Errors
`{"erro": {"codigo", "mensagem"}}`. 400 invalid body; 401 key; 402 budget; 403 model not
allowed; 413 body too large; 429 limit (client or provider); 502 invalid response/provider error;
503 breaker open or no destination; 504 timeout; 500 internal without details.

### Data
`clientes(id, nome, rpm, orcamento_micro, cache, lgpd, modelos, criado_em)`,
`chaves(id, cliente_id, prefixo, hash UNIQUE, criada_em, revogada_em)`,
`uso(ts, cliente_id, request_id, modelo, destino, status, tokens..., custo_micro, economia_micro,
latencia_ms, ttft_ms, cache, stream, shadow, pii, tamanho_resposta)`. No message content.
SQLite in WAL. Aggregates (`uso_minuto`) wait until the scan gets heavy.

### Admin and dashboard
`/admin/login` (cookie `HttpOnly; SameSite=Strict`, or `Authorization: Bearer ADMIN_TOKEN`),
`/admin/clientes` CRUD, `/admin/clientes/{id}/chaves` create/list/revoke (the full key only at
creation), `/admin/uso?de=&ate=&cliente=` (summary, per client, per model, per-minute series, p95),
`/admin/provedores` (breaker state). Dashboard: Overview, Clients and keys, Shadow,
Playground (fetch + ReadableStream with a `ck_` key pasted by the user).
`/healthz`, `/readyz`, `/metrics` (Prometheus format, hand-rolled counters).

## 4. Tests
- Table-driven: cost (peak/off-peak/prefix cache), LGPD (real cases and false positives), retry.
- Fuzz: mask → restore returns the original; stream restorer with arbitrary cuts.
- Concurrency with `-race`: singleflight N→1, cache, semaphore never exceeds the limit.
- Integration with `httptest` + the fake: 401, 429, 402, non-streaming, streaming, cache, LGPD round trip,
  fallback on 503, breaker.
- Gateway overhead benchmark (`-benchmem`). k6: constant, cache, chaos (50% 503s).

## 5. Out of scope for now (in the backlog)
Prometheus client and OpenTelemetry, Postgres and Redis, Cloud Run deploy, rate limit by estimated
tokens with reconciliation, per-client concurrency limit, semantic cache, live stream fan-out
for singleflight followers, off-peak job queue.
