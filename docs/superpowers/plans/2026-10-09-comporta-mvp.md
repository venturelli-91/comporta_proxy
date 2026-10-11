# Comporta MVP — Execution plan (v2: software house + .claude + ponytail)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans (native execution chosen by the user). Use the `- [ ]` checkboxes to track progress.
>
> Historical document: the plan as executed on 2026-10-09 (originally in Portuguese). File, package and
> commit names are the ones used at the time; several `.claude/` files were renamed to English on 10/11.

## Context

`CLAUDE.md` defines Comporta (an LLM gateway in Go) for the 2026-10-14 interview. The spec
(`docs/superpowers/specs/2026-10-09-comporta-design.md`) and plan v1 (`docs/superpowers/plans/2026-10-09-comporta-mvp.md`)
were already approved in an earlier session. Task 1 is committed (`6132d4c`). The code for Task 2
(`internal/provedor/`) exists but isn't in git. Now the user asks to implement the whole plan in automatic
mode, **one commit per feature**, following the rules of his software house (`venturelli-house/`, excluded from
git through `.git/info/exclude`), with everything documented in `.claude/` and ponytail auditing simplifications.

**User decisions (10/09):** branch `feat/mvp` + push + PR at the end (merging stays with him); bring into the
project only the software-house roles that are useful here; native execution with an independent review at the end.

**My decisions, for him to confirm:**
- Commits in **Conventional Commits in Portuguese** + `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`. The project's CLAUDE.md ("clear messages in Portuguese") and the existing commits outweigh the global rule of commits in English. (Superseded on 10/11: everything in the repository is now in English.)
- The "learning mode" from CLAUDE.md becomes documentation: each feature gets, in `.claude/docs/features/`, a suggested exercise, a comparison with TS/Node and an interview question. The user asked for automatic mode, so I write the code.
- Plan v1 keeps the contract format, without repeating the code, as agreed. This v2 replaces v1 at the same path in the repo (Task 0).
- Per the global rule, I warn the user when the context reaches 40%.

## Global Constraints (apply to every task)

- Go 1.27.2 at `C:\Users\PC\sdk\go1.27.2\bin`, which is not on the PATH. In every command: `$env:PATH="C:\Users\PC\sdk\go1.27.2\bin;$env:PATH"`.
- The race detector needs gcc on Windows, so it runs in Docker: `docker run --rm -v "${PWD}:/src" -v comporta-gomod:/go/pkg/mod -w /src golang:1.27 go test -race ./...`
- Allowed dependencies, and only these: `golang.org/x/time/rate`, `golang.org/x/sync`, `github.com/sony/gobreaker/v2`, `modernc.org/sqlite`. Front end: Vite, React, TS, TanStack Query, Recharts, Tailwind.
- Errors in the `{"erro":{"codigo","mensagem"}}` format. Money in `int64` micro-dollars. Mask before caching. Message content never goes to logs or the database. Keys stored as SHA-256 and compared with `crypto/subtle`.
- Comments explaining the why. Deliberate simplifications marked with `// ponytail:`, stating the ceiling and the upgrade path.
- The software house's autonomy matrix: merging to main, deploying and any dependency outside the list above need the user's approval.

## Review Focus (each item gets a test in its owning task)
1. The client disconnects mid-stream: the provider call is cancelled and usage is recorded with status 499 (Task 8).
2. The provider only sends `: keep-alive`: the watchdog responds 504 (Task 8).
3. The text already contains the literal `[CPF_1]`: restoring doesn't replace the user's literal (Task 6 fuzz).
4. A revoked key stops working immediately, because the snapshot is reloaded on every admin mutation (Task 8).
5. Usage channel full: the API doesn't block, drops the event and counts it (Task 7).

## Cycle of each feature (Definition of Done, repeated in `.claude/docs/checklist.md`)
1. Re-read the matching section of the architecture doc and of the spec.
2. TDD: failing test, minimal code, passing test.
3. `go vet ./...` and `gofmt -l .` with no output. Tests with `-race` passing in Docker.
4. `ponytail:ponytail-review` on the feature's diff. Apply the cuts and mark what stays with `ponytail:`.
5. Create `.claude/docs/features/NN-<name>.md` (what, why, decisions, how to test, Go vs TS, exercise, interview question). Add a line to `.claude/docs/log.md` and tick the item in `.claude/docs/backlog.md`.
6. One commit with that feature's code and documentation.

---

### Task 0: Branch, software house and `.claude/` structure
- [ ] `git switch -c feat/mvp`
- [ ] Create the structure:
```
.claude/
  README.md                 index of the folder and of how to use the agents and commands
  softhouse.md              venturelli-house rules adapted (principles, flow, routing, autonomy matrix, ponytail on every PR)
  agents/                   tech-lead, dev-backend, dev-frontend, code-reviewer (with ponytail-review), qa-automation,
                            security (LGPD/keys), performance (k6/latency), devops-sre (Docker/Cloud Run)
                            -> adapted from venturelli-house/.claude/agents/*.md for Go + React + this repo
  commands/                 ticket.md, daily-report.md, feature.md (runs the feature cycle above)
  skills/
    go-gateway/SKILL.md     the project's Go conventions + feature checklist (summarizes CLAUDE.md, doesn't duplicate it)
    interview/SKILL.md      generates interview questions and answers from .claude/docs/features
  docs/
    log.md                  journal: date, feature, commit, numbers, decisions
    backlog.md              the CLAUDE.md roadmap as a checklist (MVP / if there's time / later)
    checklist.md            DoD per feature, pre-PR and pre-interview checklists
    features/               one file per feature (created in each task)
    architecture/backend.md package map, request path, interfaces (points to the architecture doc and the spec)
    architecture/frontend.md dashboard structure, routes, state (TanStack Query), embedding in the binary
    design/dashboard.md     screens, tokens, components, empty/error states
    communication/stakeholders.md  pitch and status without jargon (non-technical)
    communication/tech-leads.md    decisions, trade-offs, risks
    communication/devs.md          onboarding: run, test, contribute
```
- [ ] Copy this plan to `docs/superpowers/plans/2026-10-09-comporta-mvp.md`, replacing v1.
- [ ] Commit `docs(claude): softhouse, agentes, skills e documentação de processo`.

### Task 1: Config and cost ✅ (`6132d4c`)
Only create `.claude/docs/features/01-config-cost.md` and the log line. Goes into the Task 0 commit.

### Task 2: Provider (OpenAI-compatible HTTP client + fake)
**Files:** `internal/provedor/{cliente,falso,provedor_test}.go` (already exist, uncommitted). Delete `internal/provedor/.impeccable/` and add `.impeccable/` to `.gitignore`.
**Produces:** `provedor.Novo(nome, baseURL, chave string, maxConexoes int, timeoutConexao time.Duration) *Cliente`; `(*Cliente).Enviar(ctx, corpo []byte) (*http.Response, error)`; `*ErroHTTP{Status, RetryAfter, Mensagem}` with `Repetivel()` and `Espera()`; `Falso{Latencia, PorToken, TaxaErro, Chamadas atomic.Int64}`, which implements `http.Handler`.
- [ ] Run the existing tests (non-streaming, streaming, 503, 429 with Retry-After, 400, network down, cancelled context).
- [ ] Feature cycle. Commit `feat(provedor): cliente compatível com OpenAI e provedor falso`.

### Task 3: Retry with backoff and jitter
**Files:** `internal/resiliencia/retry.go`, `retry_test.go`.
**Produces:** `resiliencia.Retry(ctx, tentativas int, base time.Duration, fn func() error) error`. Retries when the error implements `Repetivel() bool` and returns true; uses `Espera() time.Duration` when it's greater than zero; gives up if the context's deadline can't fit the wait.
- [ ] Table-driven: success on the 1st attempt; success on the 3rd; non-retryable error exits on the 1st; Retry-After honored; context cancelled during the wait.
- [ ] Commit `feat(resiliencia): retry com backoff exponencial, jitter e Retry-After`.

### Task 4: Router (semaphore, breaker, fallback)
**Files:** `internal/gateway/roteador.go`, `roteador_test.go`. New dependency: `github.com/sony/gobreaker/v2`.
**Consumes:** `Config`/`ModeloCfg`/`Destino` (Task 1), `provedor.Cliente` and `ErroHTTP` (Task 2), `resiliencia.Retry` (Task 3).
**Produces:** `NovoRoteador(cfg *Config, clientes map[string]*provedor.Cliente) *Roteador`; `(*Roteador).Chamar(ctx, alias string, corpo map[string]json.RawMessage) (*http.Response, Destino, error)`; `Estado() []EstadoDestino`; `ErrSemDestino`, `ErrOcupado`.
- [ ] Tests: the primary returns 503 and falls back; 400 doesn't fall back; the breaker opens and skips the destination; with the semaphore, concurrency never exceeds the limit (test with `-race`).
- [ ] Commit `feat(gateway): roteador com semáforo, circuit breaker, retry e fallback`.

### Task 5: LRU cache with TTL
**Files:** `internal/cache/cache.go`, `cache_test.go`, with `container/list` and a mutex.
**Produces:** `cache.Novo(maxBytes int64, ttl time.Duration) *Cache`; `Get(string) (Entrada, bool)`; `Set(string, Entrada)`; `Entrada{Corpo []byte; Stream bool; Custo int64}`.
- [ ] Tests: hit, TTL expiration, LRU eviction by bytes, concurrent access with `-race`.
- [ ] Commit `feat(cache): LRU com TTL e limite de memória`.

### Task 6: LGPD
**Files:** `internal/lgpd/lgpd.go`, `lgpd_test.go`.
**Produces:** `NovoMapa() *Mapa`; `Mascarar`, `Restaurar`, `Len`; `NovoStream() *Stream`; `(*Stream).Escrever(string) string`; `Finalizar() string`.
- [ ] Table-driven: CPF/CNPJ with and without punctuation, and with an invalid check digit; email; mobile and landline, with and without +55; card with valid and invalid Luhn; the same value always gets the same placeholder.
- [ ] Fuzz: `Restaurar(Mascarar(s)) == s`, including when `s` already contains a literal placeholder. In the stream with arbitrary cuts, the result equals `Restaurar`'s.
- [ ] Commit `feat(lgpd): mascaramento de CPF, CNPJ, e-mail, telefone e cartão com restauração em stream`.

### Task 7: Storage
**Files:** `internal/armazenamento/{banco,uso,banco_test}.go`. New dependency: `modernc.org/sqlite`.
**Produces:** the Task 7 contracts from plan v1 (`Abrir`, `Cliente` CRUD, `CriarChave` → `ck_...` text shown exactly once, `ChavesAtivas` as a hash → client map, batch `Gravador` with `Registrar(Evento) bool` and `Descartados()`, `Relatorio` with p95, `GastoDoMes`).
- [ ] Tests with a temporary database: CRUD; key stored only as a hash; revocation; batched writes; drop with a capacity-1 channel; report with p95.
- [ ] Commit `feat(armazenamento): SQLite com clientes, chaves por hash e uso em lote`.

### Task 8: HTTP API
**Files:** `internal/httpapi/{servidor,auth,chat,relay,admin,metricas,chat_test,bench_test}.go`, `web/embed.go` (with `web/dist/.gitkeep`). New dependencies: `x/time/rate`, `x/sync/singleflight`.
**Produces:** `httpapi.Novo(Deps) (*Servidor, error)`; `Handler() http.Handler`; `Recarregar(ctx) error`.
- [ ] Integration tests with `httptest` and the fake: 401; 429 with Retry-After; 402; 403; non-streaming response; streaming; cache hit; singleflight (N requests → 1 call, counted by `Falso.Chamadas`); LGPD round trip; fallback on 503; revocation; shadow; disconnect with 499; watchdog with 504.
- [ ] `BenchmarkOverhead` with `-benchmem`.
- [ ] Commit `feat(httpapi): chat compatível com OpenAI, admin e métricas`.

### Task 9: main + Docker
**Files:** `cmd/comporta/main.go`, `Dockerfile` (multi-stage, distroless), `docker-compose.yml`.
- [ ] Without `DEEPSEEK_API_KEY`, starts the fake on loopback. Graceful shutdown drains the writer.
- [ ] Smoke: `go run ./cmd/comporta` + `curl`, with and without streaming. `docker compose up` answers on `/healthz`.
- [ ] Commit `feat: binário comporta com shutdown gracioso e imagem distroless`.

### Task 10: React dashboard
**Files:** `web/` (Vite + React + TS + TanStack Query + Recharts + Tailwind). Design per `.claude/docs/design/dashboard.md`, using the `frontend-design` skill.
- [ ] Screens: Overview (cards, per-minute chart, per-client table), Clients and keys, Shadow, Playground with streaming.
- [ ] `npm run build` produces `web/dist`, embedded in the binary. Smoke in the browser with the binary.
- [ ] Commit `feat(web): painel com visão geral, clientes, shadow e playground`.

### Task 11: Load
**Files:** `carga/carga.js`. Runs k6 via Docker (`grafana/k6`) against the binary with the fake.
- [ ] Scenarios `sem_cache`, `com_cache` and `caos` (50% 503s). Numbers in `.claude/docs/log.md`.
- [ ] Commit `test(carga): cenários k6 contra o provedor falso`.

### Task 12: Ponytail audit, final docs and PR
- [ ] `ponytail:ponytail-audit` on the whole repo. Apply what's safe, with tests passing. Commit `refactor: simplificações apontadas pela auditoria ponytail`. If there are no cuts, just record it in the log.
- [ ] `ponytail:ponytail-debt` → `.claude/docs/backlog.md` ("Deliberate debt" section).
- [ ] Portfolio README (architecture, decisions, k6 numbers, LGPD limitations); update the roadmap and decisions in `CLAUDE.md` and the architecture doc; finalize the communication docs. Commit `docs: README, decisões e comunicação finais`.
- [ ] Independent review of the whole branch (`superpowers:requesting-code-review`). Fix what blocks.
- [ ] `git push -u origin feat/mvp` + `gh pr create` (body with summary, evidence and the 🤖 attribution line). **Don't merge.**

## End-to-end verification
- `go vet ./...` and `gofmt -l .` with no output; `go test -race ./...` green in Docker.
- `go run ./cmd/comporta`: `curl -H "Authorization: Bearer ck_..." localhost:8080/v1/chat/completions`, with and without `stream:true`. A valid CPF in the prompt comes back restored and reaches the fake masked.
- The dashboard opens at `localhost:8080`, shows usage and the playground streams.
- k6: the three scenarios run and the numbers are recorded.
- PR open on GitHub with all the per-feature commits.
