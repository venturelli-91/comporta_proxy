# Backlog

Software-house priorities: P0 (interview deadline), P1 (this week), P2 (improvement).

## Translation to English, phase 2 (P1, after 10/14)
Phase 1 (10/11) translated docs, `.claude/`, code comments, UI text and API error messages. Still in Portuguese:
- [ ] Go identifiers (types, functions, variables, test names) and package names (`provedor`, `armazenamento`, `resiliencia`)
- [ ] `comporta.json` keys (`provedores`, `modelos`, `precos`...) and model aliases (`rapido`, `forte`, `migracao`)
- [ ] API contract: error envelope `{"erro":{"codigo","mensagem"}}`, error codes (`chave_invalida`...), admin routes and JSON fields (`/admin/clientes`, `orcamento_micro`...), `X-Comporta-Cache: ligado`, `X-Comporta-Enviado`
- [ ] SQLite tables and columns (needs a migration or a recreated database)
- [ ] Environment variable `DEMO_CHAVE`, folders `carga/` and `web/src/telas/`, the public demo token/key
- [ ] Update the README, the dashboard and `carga/carga.js` together with the contract (a breaking change: version or announce it)

## Login security (P0) — solved on 10/10, branch `fix/login-seguranca`
- [x] Race (TOCTOU): `Allow()` on entry; test with 20 parallel attempts
- [x] `X-Forwarded-For` only with a trusted proxy (`K_SERVICE` or `TRUST_PROXY=1`)
- [x] Pruning of the attempts map removes only full limiters
- [x] M-5 revoking a key requires the owning client
- [x] Stdlib cuts from the ponytail audit (`cmp.Or`, `slices.SortFunc`, no `limitador` struct)

Public demo: `render.yaml` + `DEMO_CHAVE` seed (10/10). Cloud Run waits until there is billing.

## MVP — by 10/13 (P0)
- [x] 01 JSON config and cost with peak hours
- [x] 02 Provider: OpenAI-compatible client + fake provider
- [x] 03 Retry with backoff, jitter and Retry-After
- [x] 04 Router: semaphore, circuit breaker, fallback
- [x] 05 LRU cache with TTL
- [x] 06 LGPD: masking + restoring (including in streams)
- [x] 07 Storage: clients, hashed keys, batched usage
- [x] 08 HTTP API: chat (with and without streaming), auth, rate limit, budget, cache + singleflight, shadow, admin, metrics
- [x] 09 Binary + Docker + compose
- [x] 10 React dashboard
- [x] 11 k6 against the fake, numbers in the README
- [x] 12 Ponytail audit, README, PR

## After the interview (P1/P2)
- [ ] Rate limit by estimated tokens reconciled with `usage`
- [ ] Hard budget cap: pre-reserve the maximum cost and settle with the real `usage` (today it's a soft cap)
- [ ] Mask `tools`, `tool_calls[].function.arguments` and multimodal content; restore `tool_calls` in streams
- [ ] Richer shadow comparison report (content difference, not just size)
- [ ] Metrics with the official Prometheus client + OpenTelemetry
- [ ] Postgres + Redis (cache and rate limit shared across instances)
- [ ] Cloud Run deploy with Secret Manager
- [ ] Live stream fan-out for singleflight followers
- [ ] Queue of deferrable jobs to run off-peak
- [ ] Semantic cache, with a high threshold and measurement of false hits

## Deliberate debt (`// ponytail:`)
Harvested from the `ponytail:` comments on 10/09 (updated 10/11). Each line has the trigger for paying it.

| Where | Shortcut | Pay when |
|---|---|---|
| `internal/httpapi/servidor.go` | Budget (month's spend) in memory | More than one instance → Redis |
| `internal/httpapi/admin.go` | `ipDe` trusts the last XFF entry (a single proxy) | HTTPS Load Balancer in front → count N trusted proxies |
| `internal/httpapi/admin.go` | Pruning the attempts map is O(n) and only removes full ones | An attack with >10k real IPs spent at once → bounded LRU |
| `internal/httpapi/auth.go` | The same 32 concurrent requests limit for everyone | Some client needs more → a client column |
| `internal/httpapi/auth.go` | Admin sessions in memory | More than one instance → Redis |
| `internal/httpapi/auth.go` | A removed client's limiter stays in the map | Number of clients explodes → delete in removerCliente |
| `internal/httpapi/chat.go` | Estimated usage (cut stream) without a flag on the event | The dashboard needs to tell them apart → new column |
| `internal/httpapi/chat.go` | Singleflight followers get a replay, not a live stream | TTFT of cacheable requests matters → fan-out |
| `internal/httpapi/relay.go` | One LGPD restorer for all choices | A client uses `n > 1` |
| `internal/httpapi/relay.go` | Multimodal content (parts) isn't masked | A client sends images or parts |
| `internal/httpapi/metricas.go` | Hand-rolled counters, no labels | Real Prometheus/Grafana → official client |
| `internal/gateway/roteador.go` | `Estado()` without semaphore occupancy | Together with the Prometheus metrics |
| `internal/gateway/roteador.go` | A removed client's breakers stay in the map | It grows → an `Esquecer(cliente)` called by the admin |
| `internal/cache/cache.go` | Global mutex | Contention shows up in a profile → shards |
| `internal/armazenamento/uso.go` | The report scans the raw table | A slow query in a profile → `uso_minuto` |
| `.claude/docs/architecture/frontend.md` | No react-router | Nested routes or deep links |

## Minor findings from the final review (10/09) — deferred
- [x] M-1 A concurrent `Recarregar` could store an old snapshot (revocation undone until the next mutation): mutex around `Recarregar` (solved 10/10)
- [x] M-2 A usage batch that fails to write disappears without a log: log it and add to `descartados` (solved 10/10)
- [x] M-3 Shutdown waits for shadows without a limit: server context cancelled on SIGTERM + ~8 s deadline (solved 10/10)
- [x] M-4 The stream restores only `delta.content`; DeepSeek's `reasoning_content` stays masked (solved 10/10)
- [x] M-5 (solved 10/10) Revoking a key ignored the client in the path: `AND cliente_id = ?`
- [x] M-6 A non-streaming response above 16 MiB is truncated and returns 200: read `max+1` and return 502 (solved 10/10)
- [ ] M-7 The shadow screen compares SSE size × JSON size: measure the content size
- [x] M-8 (solved 10/09: random revocable session, login limit, Secure behind a proxy, compose without a default token) Admin cookie not revocable on the server, `Secure` behind Cloud Run's TLS, no attempt limit, default token in compose
- [x] M-9 The disconnect test doesn't check the 499 status explicitly (solved 10/10)
- [x] M-10 An expired session doesn't go back to login in the dashboard (invalidate `sessao` on a 401) (solved 10/10)

## Minor findings from the 10/10 review — deferred
- [x] A response above 16 MiB becomes 502 with cost 0, but the provider charged: estimate as for a cut stream (solved 10/10)
- [x] reasoning_content held in the restorer (ending in `[`) comes out after the content: finalize the reasoning on the first content chunk (solved 10/10)
- [x] The parallel login limit test should require exactly 5, not at most 5 (solved 10/10)
- [x] `ipDe` assumes Cloud Run without an HTTPS Load Balancer in front (with an LB, the last XFF entry is the LB) (documented as `ponytail:` in the code)
- [x] recarregar_test ignores CriarCliente/CriarChave errors; the false negative without the mutex depends on 200 ms (errors checked 10/10; the 200 ms wait stays)

## Findings from the full review (10/11)
- [x] Unknown `/v1/*` and `/admin/*` routes returned the dashboard's `index.html` with 200 → JSON 404
- [x] `conexao_s` missing meant no dial timeout → defaults filled in `Carregar`
- [x] Empty `AdminToken` checked on every login → refused once in `Novo`
- [x] The dashboard showed the login screen for any session error → only on 401
- [ ] Merge `resiliencia` into `gateway` (−6 lines): kept on purpose for now, "interface in the consumer" is documented and an interview talking point

## LGPD detection misses found by the corpus (10/12)
Each fix must add new corpus lines it wasn't written against, then raise the floor in `qualidade_test.go`.
- [ ] CPF written with spaces (`529 982 247 25`) is not detected
- [ ] Mobile number without area code (`98765-4321`) is not detected; risk of false positives on short numbers
- [ ] Spelled-out email (`maria arroba exemplo ponto com`) is not detected
- [ ] An order number that passes the CPF check digit (`98765432100`) is masked: the accepted cost of check-digit detection, possibly mitigated by context words ("pedido", "protocolo")
- [ ] Names and addresses: need local NER (see README, "Toward LGPD compliance")
