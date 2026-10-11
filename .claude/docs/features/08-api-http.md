# 08 — HTTP API (chat, admin, metrics)

**Package:** `internal/httpapi` · **Embedded dashboard:** `web/embed.go`

## What it does
| Route | Purpose |
|---|---|
| `POST /v1/chat/completions` | OpenAI-compatible, with and without `stream` |
| `GET /v1/models` | Aliases allowed for the key |
| `GET /healthz`, `/readyz`, `/metrics` | Probes and counters in Prometheus format |
| `/admin/*` | Login (cookie), clients, keys, usage, providers, models |
| `/` | Embedded React dashboard (falls back to `index.html`); unknown `/v1/*` and `/admin/*` routes return a JSON 404 |

Chat path: request ID → `Bearer ck_` → hash → snapshot → concurrency per client (429) → rate limit (429 + `Retry-After`) →
budget (402) → body ≤ 1 MiB (413) → alias (400/403) → LGPD → `include_usage` in streams → opt-in cache + singleflight
**or** shadow + direct call → relay → cost → usage event.

## Why this way (decisions)
- **Snapshot in an `atomic.Pointer`:** authenticating locks nothing, and the admin swaps the whole snapshot on every change. A revoked key stops working immediately (`TestRevogacaoValeNaHora`).
- **Body as `map[string]json.RawMessage`:** fields the gateway doesn't know (`tools`, `response_format`...) go to the provider untouched.
- **Stream headers only on the first useful data:** until then a real 504 can still be returned. Keep-alives don't reset the watchdog.
- **Client disconnected = 499:** the request's context goes all the way to the provider and the paid call is cancelled.
- **Singleflight with `context.WithoutCancel`:** if the "leader" disconnects, the others waiting for the same response don't lose the call.
- **Who pays in the singleflight:** the leader records the cost; the followers record savings, like a cache hit.
- **Shadow only without cache**, in a goroutine with its own context, tracked by a `WaitGroup` so shutdown waits for it.
- **The admin cookie is a random session:** the server keeps only its SHA-256 with the expiry, and logout revokes it. `HttpOnly; SameSite=Strict`; the token is compared with `subtle.ConstantTimeCompare`, and an empty token is refused at startup.
- **The `ck_` key is looked up by its hash in a map:** the lookup time doesn't leak the key, because the attacker doesn't control the hash.
- Ponytail: singleflight followers get a replay instead of a live stream; metrics without the Prometheus client; multimodal content isn't masked; one stream restorer for all choices. All marked in the code.

## Numbers (10/09, Aurélio's laptop, fake provider on loopback)
`go test -run x -bench Overhead -benchmem ./internal/httpapi/`
| Case | ns/op | B/op | allocs/op |
|---|---|---|---|
| direct | ~187,000 | 27,238 | 260 |
| with LGPD | ~215,000 | 38,231 | 305 |

That includes the real HTTP hop to the fake. Masking costs about 28 µs per request.

## How to test
`go test ./internal/httpapi/ -v`: 401, 429 + Retry-After, 402, 403, 400, 413, non-streaming, streaming, cache hit, opt-in cache,
singleflight (10 → 1), LGPD round trip (with and without streaming), fallback, revocation, shadow, disconnect (499), watchdog (504),
`/v1/models`, `/metrics`, admin without a token and JSON 404 for unknown API routes. Mutation testing confirmed that the LGPD and
singleflight tests detect the regression.

## Go vs TypeScript
- `http.ServeMux` with patterns like `"POST /v1/chat/completions"` and `{id}` (Go 1.22+) replaces Express's router. The most specific pattern wins, so `/v1/` beats `/`.
- Middleware is just a function that takes and returns an `http.HandlerFunc`, with no `next()`.
- `context.WithValue` is like Express's `res.locals`, but immutable and chained.
- `http.NewResponseController(w).Flush()` corresponds to `res.flush()` for SSE in Node.

## Exercise
Write the test that a client with an exhausted budget **can't** get through the cache (the 402 comes first). Then move the budget check after the cache and watch the test break.

## Interview question
"Ten identical requests arrive at the same time and the client of the first one disconnects. What happens to the other nine?"

## Fixes from the final review (10/09)
The independent review of the branch found 7 important issues. All were fixed with a test that failed first:
| Finding | Fix | Test |
|---|---|---|
| A stream hung without a deadline until the provider sent headers | `vigia` (watchdog): it starts before `Chamar` and cancels with `errTimeout` | `TestWatchdogAntesDosCabecalhos` |
| Cache/singleflight path without a watchdog | the leader reads through the same `vigia` | `TestWatchdogNoCaminhoDoCache` |
| The breaker ignored timeouts | only `context.Canceled` (client) is excluded; `Enviar` returns `context.Cause` | `TestBreakerContaTimeout` |
| A literal placeholder in one message collided with real data in another | `lgpd.MascararVarios` reserves the text of every message | `TestMascararVariosNaoColideEntreMensagens` + `FuzzMascararVarios` |
| A usage chunk with `choices: []` broke the SDKs' `chunk.choices[0]` | relayed only if the client asked for `include_usage`; the fake now mimics DeepSeek | `TestUsageSoQuandoOClientePede` |
| A cut stream recorded a cost of 0 (bypassing the budget) | estimate: message bytes / 4 + 1 token per chunk | `TestStreamCortadoAindaCobra` |
| Unbounded shadows competed for the account's concurrency | a 16-shadow semaphore; without a slot, drop and count (`comporta_shadow_descartados_total`) | `TestShadowLimitado` |

A bonus of the fix: the relay got simpler. The `vigia` cancels the context and reading became synchronous, without the reader goroutine.
