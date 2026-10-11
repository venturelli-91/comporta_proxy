# Backend architecture

Source of truth for decisions: `docs/ARCHITECTURE.md`. Contracts: spec and plans in `docs/superpowers/`.
This page is the map for navigating the code. Package names are still in Portuguese (English in parentheses).

## Packages
| Package | Responsibility | Depends on |
|---|---|---|
| `cmd/comporta` | Reads the environment, wires dependencies, starts the fake if there is no `DEEPSEEK_API_KEY`, graceful shutdown | all |
| `internal/gateway` | Config (`comporta.json`, defaults filled in `Carregar`), cost, router (semaphore → breaker → retry → fallback) | `provedor`, `resiliencia`, gobreaker |
| `internal/provedor` (provider) | OpenAI-compatible HTTP `Cliente` (DeepSeek or another) + `Falso` (an `http.Handler` speaking the same protocol) | stdlib |
| `internal/resiliencia` (resilience) | `Retry` with exponential backoff, jitter and `Retry-After` | stdlib |
| `internal/cache` | LRU with TTL and a byte limit | stdlib |
| `internal/lgpd` | Mask and restore CPF, CNPJ, email, phone and card; stream restorer | stdlib |
| `internal/armazenamento` (storage) | SQLite: clients, keys (hashed), batched usage, reports | modernc sqlite |
| `internal/httpapi` | Routes, auth, admission, chat (with and without streaming), admin, metrics, embedded dashboard | everything above, x/time/rate, x/sync |

The graph is acyclic: `httpapi` → `gateway` → `provedor`/`resiliencia`. `resiliencia` doesn't import
`provedor`. It only sees the `Repetivel()` (retryable) and `Espera()` (wait) interfaces, which `ErroHTTP` implements.

## Path of `POST /v1/chat/completions`
```
recover + X-Request-Id
 → auth: Bearer ck_... → sha256 → client (atomic.Pointer snapshot)
 → admission: concurrency per client (429) · token bucket (429 + Retry-After) · month's budget (402)
 → body ≤ 1 MiB → map[string]json.RawMessage · known model (400) · model allowed (403)
 → LGPD: masks messages[].content (if the client has it on)
 → opt-in cache: key sha256(client + masked body); hit → respond
 → singleflight on the same key (cacheable requests only)
 → router: destinations [primary, fallback...]; each one: semaphore → breaker → retry
 → relay: whole JSON or SSE line by line (ignores keep-alives, restores PII, captures usage)
 → usage event → channel → batch writer (SQLite)
 → shadow (if configured): goroutine with its own context, records the comparison
```

## Concurrency
| State | Protection |
|---|---|
| Client and key snapshot | `atomic.Pointer`, swapped whole on every admin mutation |
| Per-client limiters and open requests | `sync.Mutex` + maps (lazy creation) |
| Cache | `sync.Mutex` (the LRU touches the list even on Get) |
| Per-model semaphore | buffered channel |
| Metric counters | `atomic.Int64` |
| Usage | buffered `chan Evento`; drops when full |
