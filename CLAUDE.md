# CLAUDE.md — Comporta: an LLM gateway in Go

## Context

Aurélio's portfolio project for an interview at Globo on 2026-10-14. The portfolio is international:
**everything committed to the repository is in English** (docs, comments, UI, commit messages).
Talk to me in Portuguese in the chat.

The job is for a **Go proxy on GCP** that is replacing a legacy application: it receives requests from
several client web applications and routes data lookups across clouds more efficiently.

I come from **TypeScript, React and Python** and I'm learning Go now. The project exists so that:
1. **I really learn Go** and can explain every decision in the interview.
2. I have a public, working product with measured numbers that solves a real business problem.

## The product

**Comporta** is an *LLM gateway*: a proxy that sits between a company's applications and AI providers
(DeepSeek first, then any OpenAI-compatible API).

**Business problem:** companies that put AI into their products quickly lose control. The API key is spread
across several apps, nobody knows how much each team spends, the same question is paid for many times, one
buggy app burns the month's budget and, when the provider goes down, everything goes down with it.
Tools like LiteLLM, Portkey, Helicone and Cloudflare AI Gateway exist precisely for this.

**What Comporta does:**
- Each client application gets **its own key** (`ck_...`); the real provider key stays only on the server.
- **Per-client rate limit and budget** (requests/minute and a US$ spend limit per month).
- **Cache** of identical responses + **singleflight** (identical concurrent requests become a single call).
- **Timeout, retry with backoff and circuit breaker** per provider; **fallback** to another model/provider.
- **Streaming** (SSE) relayed to the client token by token.
- **Usage records**: tokens, estimated cost, latency, cache hit, per client and per model.
- **Shadow mode**: sends the same request to a second model, returns only the primary's answer and records
  the comparison (cost, latency, response size). It lets you migrate model/provider without risk, which is
  the same problem as "modernizing a legacy system".
- **Personal data protection (LGPD, Brazil's data protection law)**: before sending to the provider, detects
  and masks CPF, CNPJ, email, phone numbers with area code and cards (`"meu CPF é 123.456.789-09"` →
  `"meu CPF é [CPF_1]"`), and restores the original values in the response. On/off per client. It matters
  even more with providers hosted outside Brazil, such as DeepSeek.
- **React dashboard**: clients, keys, usage, cost, cache hit rate, p95 latency, how much personal data was
  masked, and a playground.

**Parallel with the job:** a Go proxy receiving requests from several client applications, routing to
external backends efficiently (cache, deduplication, reused connections, timeouts), with safe migration via
shadow. It's the same kind of problem, in a domain that is easy to demo.

> **Detailed architecture and technical decisions: `docs/ARCHITECTURE.md`. Read it before implementing any
> part of the request path, and update it when a decision changes.**

## Stack

- **Back end:** Go 1.24+, preferring the standard library (`net/http` with 1.22 routing, `log/slog`,
  `encoding/json`). Allowed dependencies: `golang.org/x/time/rate`, `golang.org/x/sync` (singleflight,
  errgroup), `github.com/sony/gobreaker/v2`, a database driver. Anything else: ask me first.
- **Database:** cgo-free SQLite (`modernc.org/sqlite`); a repository interface only when Postgres arrives.
- **Front end:** React + TypeScript + Vite in `web/`, TanStack Query, Recharts, Tailwind.
  The build is embedded in the Go binary with `//go:embed` (a single deploy artifact).
- **LLM:** DeepSeek through its OpenAI-compatible API (as of 2026-10-09: `deepseek-flash` and
  `deepseek-v4-pro`, reasoning on by default, cheaper off-peak). Models, prices and peak windows come from
  configuration; never hard-coded.
- **Load tests:** k6 in `carga/`, **always against the fake provider** (so no money is spent).
- **Infra:** multi-stage Docker + docker compose. Future deploy: Cloud Run (GCP), since it's the job's environment.

## Architecture

```
client apps ──(key ck_...)──▶ Comporta (Go)
                               ├─ middleware: auth → rate limit → budget → log/metrics
                               ├─ LGPD masker (outbound) / restorer (inbound)
                               ├─ cache (memory, TTL) + singleflight
                               ├─ model router (primary, fallback, shadow)
                               ├─ providers: DeepSeek | OpenAI-compatible | Fake (tests)
                               │     each with timeout, retry and circuit breaker
                               └─ usage records (SQLite, batched writes)
React dashboard ──▶ /admin/* (dashboard API, protected by an admin token/session)
```

## Folder structure

Go identifiers and package names are still in Portuguese (translation phase 2, after the interview; see
`.claude/docs/backlog.md`).

```
cmd/comporta/           minimal main: config from env, wires dependencies, graceful shutdown
internal/gateway/       domain: config, cost, router
internal/provedor/      provider: OpenAI-compatible client + fake provider
internal/cache/         TTL cache
internal/lgpd/          detection and masking of Brazilian personal data (with check-digit validation)
internal/resiliencia/   resilience: retry with backoff/jitter
internal/armazenamento/ storage: SQLite (clients, hashed keys, usage)
internal/httpapi/       public routes (/v1/...) and admin routes (/admin/...), middlewares, SSE
web/                    React dashboard
carga/                  k6 scripts (load tests)
```

## API

- `POST /v1/chat/completions` — **compatible with the OpenAI format** (so any existing SDK works by changing
  only the base URL). Supports `stream: true`.
- `GET  /v1/models` — models allowed for that client.
- `GET  /healthz`, `GET /readyz`, `GET /metrics`.
- `/admin/clientes` (CRUD), `/admin/clientes/{id}/chaves` (create/revoke keys), `/admin/uso?de=&ate=&cliente=` (usage).

Errors always in the format `{"erro": {"codigo": "...", "mensagem": "..."}}` (field names stay in Portuguese
until phase 2, since they are API contract; the `mensagem` text is in English), with the right status:
401 (invalid key), 429 (rate limit, with `Retry-After`), 402 (budget exhausted),
502/503/504 (provider trouble), 500 (internal, no details).

## Commands

```bash
go run ./cmd/comporta                 # API on :8080 (uses the fake provider if DEEPSEEK_API_KEY is not set)
go vet ./...
go test -race ./...                   # ALWAYS with -race
cd web && npm run dev                 # dashboard on :5173
k6 run carga/carga.js                 # against the fake provider
docker compose up
```

Variables: `PORT`, `DEEPSEEK_API_KEY`, `DEEPSEEK_BASE_URL`, `ADMIN_TOKEN`, `DB_PATH`, `TRUST_PROXY` (=1 behind
a trusted proxy; on Cloud Run `K_SERVICE` is enough), `DEMO_CHAVE` (public demo only: creates the demo client
with this key when the database is empty).

## Security rules (non-negotiable)

- The `DEEPSEEK_API_KEY` **never** goes to the front end, to logs or to the repository. `.env` is in `.gitignore`.
- Client keys stored only as a **hash** (SHA-256); the full key appears exactly once, at creation.
- Key/token comparison with `crypto/subtle.ConstantTimeCompare`.
- Don't log the content of user messages by default (only size, tokens, model). Treat it as personal data (LGPD).
- Request body size limit.
- The `[CPF_1] → real value` map lives only in the request's memory; it is never written to logs, cache or database.
- Masking happens **before** the cache: the cache key never contains personal data.

## LGPD masking: rules

- CPF and CNPJ only count as detected if the **check digit is valid** (avoids masking random numbers).
  Accept them with and without punctuation.
- Brazilian phone numbers with area code (landline and mobile, with and without +55), email and cards
  (with a Luhn check).
- The same value always gets the same placeholder within a request (`[CPF_1]` appears twice if the CPF
  appears twice), so the model stays coherent.
- Restoration in the response, including in **streaming**: a placeholder can arrive split across two chunks
  (`[CP` + `F_1]`), so the restorer needs a small buffer.
- Table-driven tests with many real cases and false positives, plus a **fuzz test** guaranteeing that
  masking then restoring returns the original text.
- Honest limitation for the README: pattern detection doesn't catch everything (names, addresses). It's a
  risk-reduction layer, not a compliance guarantee.

## Go conventions

- `gofmt`; no unused imports or variables.
- Errors as values: `fmt.Errorf("context: %w", err)`, `errors.Is`/`errors.As`, no panic for expected errors.
- Small interfaces, defined in the package that uses them ("accept interfaces, return structs").
- `context.Context` as the first parameter of everything that does I/O, and passed along
  (if the client disconnects, the provider call is cancelled).
- One reused `http.Client` per provider, with timeouts; always close `resp.Body`.
- Shared state with mutex/atomic and covered by a concurrent test with `-race`.
- Table-driven tests with `t.Run`; handlers with `httptest`; the real provider is tested against an
  `httptest.NewServer` that mimics the API, never against the real API in tests.
- Structured logs with `slog`.

## How you should work with me (learning mode)

- **Explain before writing**: 2–3 sentences on what you'll do and why, comparing with TypeScript/Node when it helps.
- **Let me write parts.** When the task is a good exercise (middleware, table-driven test, circuit breaker,
  singleflight), propose the signature and the tests and ask me to implement it. Then review it like a code review.
- Code comments in English, explaining the "why".
- Point out when something works but isn't idiomatic Go.
- At the end of each task, suggest **one interview question** about what we just did.
- Small steps, one commit per step, clear messages in English.
- Don't make up how Globo does things internally. Only cite what is public.

## Roadmap

**MVP by 10/13 — done on 10/09 (branch `feat/mvp`), one commit per feature:**
- [x] Fake provider (configurable latency and failures) + OpenAI-compatible client (DeepSeek)
- [x] OpenAI-compatible `POST /v1/chat/completions`, with and without streaming
- [x] Per-client keys (hashed in SQLite) + authentication middleware
- [x] Per-client rate limit (token bucket)
- [x] TTL cache + singleflight, with a test proving 1 call for N identical concurrent requests
- [x] Timeout + retry with backoff/jitter + circuit breaker on the provider
- [x] Usage records (tokens, estimated cost, latency, cache hit), written in batches
- [x] React dashboard: usage per client, cache, cost, p95 latency, shadow, playground
- [x] k6 against the fake provider: numbers in the README
- [x] Portfolio README with screenshot, architecture, decisions and numbers
- [x] LGPD masking with table-driven + fuzz tests, **including streaming**
- [x] Monthly budget per client (402), fallback between destinations and shadow mode with comparison

**After the interview:** see `.claude/docs/backlog.md` (includes the deliberate `ponytail:` debt).
- [ ] Translation phase 2: Go identifiers, package names, `comporta.json` keys, JSON fields and API error codes
- [ ] Prometheus metrics with labels + OpenTelemetry tracing
- [ ] Postgres + Redis (cache, rate limit and budget shared across instances)
- [ ] Cloud Run deploy with Secret Manager
- [ ] Rate limit by estimated tokens, budget alerts, semantic cache (with measurement)

## Decisions (record new ones here, with the why)

- **OpenAI-compatible API:** frictionless adoption; the client only changes the base URL.
- **Fake provider from day one:** tests and load without cost, without network and deterministic.
- **Exact, opt-in cache** (`X-Comporta-Cache` header or the client's policy): with reasoning on by default,
  `temperature` doesn't make the response deterministic, so reusing it has to be an explicit decision.
  DeepSeek's prefix cache is preserved and measured.
- **Retry and fallback only before the first byte** is sent to the client.
- **Per-model concurrency semaphore** below the DeepSeek account limit.
- **Usage written in batches:** no database write on the critical path of each request.
- **Mask before caching:** the cache and the logs never see personal data.
- **Validate the CPF/CNPJ check digit:** fewer false positives than a plain regex.
- **One OpenAI-compatible HTTP client, no `Provider` interface:** DeepSeek and "openaicompat" differ only in
  base_url; the fake is an `http.Handler` speaking the same protocol and exercises the real network code.
- **No `Repository` interface:** a single implementation (SQLite). The interface is born in the consumer
  when Postgres arrives.
- **Retry inside the circuit breaker; client 4xx doesn't open the breaker; cancellation is excluded.**
- **An LGPD placeholder never collides with user text** (a literal `[CPF_1]` → the real CPF becomes `[CPF_2]`):
  guarantees `Restore(Mask(s)) == s`, proven by fuzzing.
- **Shadow only on the uncached path:** comparing a reused response measures nothing and pays for a call.
- **Singleflight with `context.WithoutCancel`:** the leader disconnecting doesn't bring down whoever is waiting for the same response.
- **The admin cookie is a random session** (the server keeps only the hash, logout revokes it), `HttpOnly; SameSite=Strict`; the token is compared in constant time.
- **Login limited per IP, consumed on entry:** `Allow()` checks and spends in a single step (no race between parallel attempts).
- **XFF only behind a trusted proxy** (`K_SERVICE`/`TRUST_PROXY=1`): without a proxy the header belongs to the client and would give it a fresh bucket on every attempt.
- **Public demo on Render, not Cloud Run:** free tier without a card; Cloud Run requires billing. The demo uses only the fake provider, with a public token and key (disposable instance, ephemeral disk recreated by `DEMO_CHAVE`).
- **Config defaults filled in one place (`Carregar`):** whoever uses the config gets it complete; hand-built configs in tests declare their timeouts explicitly.
- **Budget is a soft cap; shadow cost stays outside the client's budget:** see `docs/ARCHITECTURE.md` §8.
- **LGPD detection measured on a labeled corpus, not tuned to it:** precision/recall per type gated with a floor at the measured values; a fix for a miss comes with new corpus lines it wasn't written against.
- **Everything in the repository is in English** (international portfolio): docs, comments, UI and
  Conventional Commits in English. Identifiers follow in phase 2. This overrides the earlier Portuguese rule.

## 30-second interview pitch

The interview at Globo is in Portuguese, so the pitch I'll say is kept in Portuguese, with the English version below.

"Construí em Go um gateway que deixa empresas brasileiras usarem IA sem vazar dado pessoal e com
custo sob controle. Ele mascara CPF, e-mail e telefone antes de enviar ao provedor e restaura na
volta. Cada aplicação tem sua chave, limite e orçamento; requisições iguais são cacheadas e deduplicadas com singleflight;
o provedor é protegido por timeout, retry e circuit breaker; e um modo shadow permite migrar de
modelo comparando os dois sem afetar o usuário. Medi com k6: 1.000 req/s com p95 de 61,7 ms contra
um provedor de 60 ms (1,7 ms de overhead), o cache evitou 99,9% das chamadas pagas num cenário repetitivo, e
com o provedor falhando metade das vezes 98,4% das requisições terminaram bem. É o mesmo tipo de problema de um proxy que atende várias
aplicações clientes e precisa buscar dados externos de forma eficiente."

"I built a Go gateway that lets Brazilian companies use AI without leaking personal data and with costs
under control. It masks CPF, email and phone numbers before sending to the provider and restores them on the
way back. Each application has its own key, limit and budget; identical requests are cached and deduplicated
with singleflight; the provider is protected by timeouts, retries and a circuit breaker; and a shadow mode
lets you migrate models by comparing both without affecting the user. I measured it with k6: 1,000 req/s with
a p95 of 61.7 ms against a 60 ms provider (1.7 ms of overhead), the cache avoided 99.9% of paid calls in a
repetitive scenario, and with the provider failing half the time 98.4% of requests succeeded. It's the same
kind of problem as a proxy serving many client applications that needs to fetch external data efficiently."
