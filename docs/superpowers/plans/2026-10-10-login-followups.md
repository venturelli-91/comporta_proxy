# Post-MVP follow-ups (login security + cleanups): implementation plan

> **For agents:** REQUIRED SUB-SKILL: superpowers:executing-plans (recommended here) or superpowers:subagent-driven-development. Steps with checkboxes (`- [ ]`).
>
> Historical document: the plan as executed on 2026-10-10 (originally in Portuguese, file name
> `2026-10-10-pendencias-login.md`). Code snippets show the code as it was then; identifiers and
> user-facing strings were still in Portuguese at the time.

**Goal:** close the 3 login security alerts, fix M-5 and apply the ponytail-audit cuts. All in a new branch with a PR to `main`, in time for the 10/14 interview.

**Architecture:** these are targeted changes in `internal/httpapi` (login, IP, limiter) and `internal/armazenamento` (revoke key). No new package. The only new config is `Deps.ConfiarProxy` (trust proxy), turned on in `main.go` by `K_SERVICE` (Cloud Run) or `TRUST_PROXY=1`.

**Tech Stack:** Go 1.26 with the stdlib and `golang.org/x/time/rate`. SQLite via `modernc.org/sqlite`.

**Spec:** top of `.claude/docs/backlog.md` ("Next session — start here (P0)"), item M-5 and this session's ponytail-audit/debt report.

## Context
PR #1 (MVP) has already been **merged**. `origin/main` is only missing the docs commit `7ecc384`. Three flaws remain in the login limit (`admin.go`):
- **Race:** the code checks `Tokens()` and only consumes afterwards.
- **Forgeable `X-Forwarded-For`:** without a proxy in front, each attempt can arrive with a new IP.
- **A forceable reset of the IP map.**

Also missing:
- **M-5:** revoking a key ignores the client in the path.
- Stdlib cuts (~20 lines).
- An outdated backlog.

## Global Constraints
- `gofmt`, clean `go vet` and `go test -race ./...`. On Windows `-race` only runs via Docker (Task 7).
- Commits in Portuguese, in the Conventional Commits format `type(scope): description`, one per task, with the line `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`. (Superseded on 10/11: commits are now in English.)
- Comments explaining the why. A deliberate shortcut gets `// ponytail: <ceiling>, <trigger>`.
- No new dependency.
- Learning mode: in **Task 1 Aurélio implements**. I write the test, he writes the code, I review.

## Review Focus
1. **Legitimate admin behind Cloud Run:** with `ConfiarProxy` on, the IP comes from the last XFF entry. The admin on another IP keeps logging in even with the attacker blocked. Test in Task 1.
2. **A correct login now consumes a token:** 5 correct logins in a row from the same IP → the 6th gets 429. Acceptable, because the limit is per IP and the attacker doesn't affect the admin. It must be documented in the Task 1 test.
3. **XFF with spaces or a single entry** (`"203.0.113.9"`, `"a, b"`): returns the last one, without spaces. Test in Task 2.
4. **Map above 10k with every limiter spent:** pruning deletes nothing, the map grows and each login scans O(n). Stays as a known limit, with `ponytail:` in Task 3.
5. **Revoking another client's key through the path** → 404, and the key stays active. Test in Task 4.

---

### Task 0: Branch
- [ ] `git switch -c fix/login-seguranca` from the current `feat/mvp` (it carries `7ecc384`).
- [ ] Copy this plan to `docs/superpowers/plans/` and commit `docs(plano): pendências de segurança do login`.

### Task 1: Consume the limit on entry (race) — **Aurélio implements**

**Files:** Modify `internal/httpapi/admin.go:35-48`; Test `internal/httpapi/chat_test.go` (near line 627).

**Interfaces:** no signature change.

- [ ] **Step 1: a parallel test that fails.** Add `sync/atomic` to the imports if missing.
```go
// 20 wrong attempts at the same time from the same IP: at most 5 may reach the comparison.
// With check-then-consume, all of them passed the check before any consumed.
func TestLoginParaleloRespeitaLimite(t *testing.T) {
	c := novoCenario(t)
	var wg sync.WaitGroup
	var comparadas atomic.Int32
	for range 20 {
		wg.Go(func() {
			req, _ := http.NewRequest("POST", c.url+"/admin/login", strings.NewReader(`{"token":"chute"}`))
			req.Header.Set("X-Forwarded-For", "203.0.113.9")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err) // t.Fatal can't be called outside the test goroutine
				return
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized {
				comparadas.Add(1)
			}
		})
	}
	wg.Wait()
	if n := comparadas.Load(); n > 5 {
		t.Fatalf("%d attempts reached the comparison; the limit is 5", n)
	}
}
```
- [ ] **Step 2: adjust `TestLoginLimitaTentativasSemTrancarOAdmin`.** Delete the final loop "a correct login must not consume the limit" and the comment "only wrong attempts count". Instead, the comment says: "the limit is per IP; a correct login also consumes, but an attacker on another IP doesn't lock the admin out".
- [ ] **Step 3:** `go test ./internal/httpapi -run 'TestLogin' -count=5`. Expected: the parallel test FAILS (more than 5).
- [ ] **Step 4: implementation (Aurélio).** Code expected at review:
```go
func (s *Servidor) login(w http.ResponseWriter, r *http.Request) {
	// Consume on entry: checking and consuming in a single step closes the race between parallel attempts.
	// A correct login also spends a token; since the limit is per IP, the attacker doesn't lock the admin out.
	if !s.limiteLogin(s.ipDe(r)).Allow() {
		retryAfter(w, time.Minute)
		erro(w, http.StatusTooManyRequests, "muitas_tentativas", "muitas tentativas de login, aguarde um minuto")
		return
	}
	var v struct{ Token string }
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&v)
	if s.d.AdminToken == "" || !iguais(v.Token, s.d.AdminToken) {
		erro(w, http.StatusUnauthorized, "nao_autorizado", "token de admin inválido")
		return
	}
	// ... the rest unchanged
```
(`s.ipDe` is only born in Task 2. In this task it stays `ipDe(r)`.)
- [ ] **Step 5:** `go test ./internal/httpapi -count=5` must PASS.
- [ ] **Step 6:** commit `fix(httpapi): limite de login consumido na entrada, sem corrida`.

### Task 2: XFF only with a trusted proxy

**Files:** Modify `internal/httpapi/servidor.go` (`Deps`), `internal/httpapi/admin.go:73-87` (`ipDe` becomes a method), `internal/httpapi/chat_test.go:119` (`novoCenario`), `cmd/comporta/main.go` (building `Deps`). Test: `internal/httpapi/chat_test.go`.

**Interfaces:**
- Produces `Deps.ConfiarProxy bool`.
- Produces `func (s *Servidor) ipDe(r *http.Request) string`.

- [ ] **Step 1: table-driven test (fails because it doesn't compile).**
```go
// Without a trusted proxy in front, XFF belongs to the client: changing it can't buy a fresh bucket.
func TestIPDe(t *testing.T) {
	casos := []struct {
		nome    string
		confiar bool
		xff     string
		quer    string
	}{
		{"sem proxy ignora xff", false, "1.2.3.4, 203.0.113.9", "192.0.2.1"}, // httptest's RemoteAddr
		{"cloud run usa a última", true, "1.2.3.4,  203.0.113.9", "203.0.113.9"},
		{"cloud run entrada única", true, "203.0.113.9", "203.0.113.9"},
		{"cloud run sem xff", true, "", "192.0.2.1"},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			s := &Servidor{d: Deps{ConfiarProxy: tc.confiar}}
			r := httptest.NewRequest("POST", "/admin/login", nil)
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := s.ipDe(r); got != tc.quer {
				t.Fatalf("ipDe = %q, want %q", got, tc.quer)
			}
		})
	}
}
```
- [ ] **Step 2:** `go test ./internal/httpapi -run TestIPDe` → FAILS (doesn't compile).
- [ ] **Step 3: implementation.**
  - In `Deps`:
    ```go
    // ConfiarProxy: there is a proxy in front (Cloud Run) that appends the real IP to X-Forwarded-For.
    // Without it, the header belongs to the client and identifies nobody.
    ConfiarProxy bool
    ```
  - In `admin.go`, `ipDe` becomes:
    ```go
    // ipDe uses the last X-Forwarded-For entry, the one the trusted proxy appends (the client
    // can only forge the earlier ones), and only when that proxy exists. Otherwise, the connection address.
    func (s *Servidor) ipDe(r *http.Request) string {
    	if xff := r.Header.Get("X-Forwarded-For"); s.d.ConfiarProxy && xff != "" {
    		return strings.TrimSpace(xff[strings.LastIndex(xff, ",")+1:])
    	}
    	host, _, err := net.SplitHostPort(r.RemoteAddr)
    	if err != nil {
    		return r.RemoteAddr
    	}
    	return host
    }
    ```
    The call in `login` becomes `s.ipDe(r)`.
  - In `novoCenario`, add `ConfiarProxy: true` to `Deps{...}`, with the comment `// the tests simulate Cloud Run: XFF separates the IPs`.
  - In `main.go`, add to `httpapi.Deps{...}`:
    ```go
    ConfiarProxy: os.Getenv("K_SERVICE") != "" || os.Getenv("TRUST_PROXY") == "1",
    ```
- [ ] **Step 4:** `go test ./internal/httpapi` → PASSES (includes the Task 1 login tests).
- [ ] **Step 5:** commit `fix(httpapi): X-Forwarded-For só vale atrás de proxy confiável`.

### Task 3: Prune only the full limiters

**Files:** Modify `internal/httpapi/admin.go:56-71`. Test: `internal/httpapi/admin_test.go` (a new, small file, `package httpapi`, to test the internal map).

- [ ] **Step 1: a failing test.**
```go
package httpapi

import (
	"strconv"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// Going past 10k IPs must not give attempts back to whoever already spent theirs.
func TestLimiteLoginPodaSoOsCheios(t *testing.T) {
	s := &Servidor{tentativas: map[string]*rate.Limiter{}}
	for i := range 10_000 {
		s.tentativas[strconv.Itoa(i)] = rate.NewLimiter(rate.Every(12*time.Second), 5)
	}
	gasto := rate.NewLimiter(rate.Every(12*time.Second), 5)
	for range 5 {
		gasto.Allow()
	}
	s.tentativas["203.0.113.9"] = gasto

	s.limiteLogin("198.51.100.7") // 10,001 entries: triggers the pruning

	if s.tentativas["203.0.113.9"] != gasto {
		t.Fatal("the spent limiter was deleted: the attacker got fresh attempts")
	}
	if n := len(s.tentativas); n != 2 {
		t.Fatalf("%d limiters left; want only the spent one and the new one", n)
	}
}
```
- [ ] **Step 2:** `go test ./internal/httpapi -run TestLimiteLoginPodaSoOsCheios` → FAILS (the spent one is deleted).
- [ ] **Step 3: implementation.** Replace the reset with:
```go
// limiteLogin: 5 attempts per IP, a new one every 12 s. Per IP so that an attacker can't
// lock the real admin out from outside.
// ponytail: pruning scans the whole map and only removes full ones; with 10k IPs spent at once
// the map grows and each login becomes O(n). A bounded LRU if this becomes a target.
func (s *Servidor) limiteLogin(ip string) *rate.Limiter {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	if len(s.tentativas) > 10_000 {
		for k, l := range s.tentativas {
			if l.Tokens() >= float64(l.Burst()) { // full = same as a new one, holds nothing
				delete(s.tentativas, k)
			}
		}
	}
	// ... the rest unchanged
```
- [ ] **Step 4:** `go test ./internal/httpapi` → PASSES.
- [ ] **Step 5:** commit `fix(httpapi): mapa de tentativas poda só limitadores cheios`.

### Task 4: M-5, revoking respects the client in the path

**Files:** Modify `internal/armazenamento/banco.go:168-171`, `internal/httpapi/admin.go:202-206`. Test: `internal/armazenamento/banco_test.go:72`.

**Interfaces:** `func (b *Banco) RevogarChave(ctx context.Context, clienteID, id int64) error`. Returns `ErrNaoEncontrado` (not found) if the key doesn't belong to that client.

- [ ] **Step 1: test.** In `TestRevogarChave`, change the call to `b.RevogarChave(ctx, c.ID, chave.ID)` and add at the start:
```go
	outro, _ := b.CriarCliente(ctx, Cliente{Nome: "outro"})
	if err := b.RevogarChave(ctx, outro.ID, chave.ID); !errors.Is(err, ErrNaoEncontrado) {
		t.Fatalf("revoking through another client's route: err = %v, want ErrNaoEncontrado", err)
	}
	if ativas, _ := b.ChavesAtivas(ctx); len(ativas) != 1 {
		t.Fatal("the key was revoked through another client's route")
	}
```
(Add `errors` to the import if missing.)
- [ ] **Step 2:** `go test ./internal/armazenamento -run TestRevogarChave` → FAILS (doesn't compile).
- [ ] **Step 3: implementation.**
```go
func (b *Banco) RevogarChave(ctx context.Context, clienteID, id int64) error {
	res, err := b.db.ExecContext(ctx, `UPDATE chaves SET revogada_em = ? WHERE id = ? AND cliente_id = ? AND revogada_em IS NULL`,
		time.Now().Unix(), id, clienteID)
	return linhaAfetada(res, err, "revogando chave")
}
```
In the handler:
```go
func (s *Servidor) revogarChave(w http.ResponseWriter, r *http.Request) {
	cliente, ok := idDe(w, r, "id")
	if !ok {
		return
	}
	if id, ok := idDe(w, r, "chave"); ok {
		s.mutou(w, r, s.d.Banco.RevogarChave(r.Context(), cliente, id), http.StatusNoContent, nil)
	}
}
```
- [ ] **Step 4:** `go test ./...` → PASSES (the HTTP revoke test in `chat_test.go:~420` uses the right path).
- [ ] **Step 5:** commit `fix(armazenamento): revogar chave exige o cliente dono`.

### Task 5: Ponytail-audit cuts

**Files:**
- `internal/httpapi/servidor.go` (`padrao`, `limitador`, `erro`)
- `internal/httpapi/auth.go:43-56` (`permitir`)
- `cmd/comporta/main.go:34-39` (`env`)
- `internal/gateway/roteador.go:195`

These are refactors without behavior changes. Coverage comes from the existing tests.

- [ ] `padrao(...)` → `cmp.Or(t.PrimeiroToken(), 30*time.Second)` (same for `EntreChunks` and `Total`). Delete the `padrao` function. Accepted difference: a negative duration in the config would no longer fall back to the default; the config is in ms and doesn't produce negatives.
- [ ] `env(n, p)` in `main.go` → `cmp.Or(os.Getenv(n), p)` in the 4 calls. Delete the `env` function.
- [ ] Delete `type limitador`. The field becomes `limites map[int64]*rate.Limiter` and `permitir` becomes:
```go
	l, ok := s.limites[cli.ID]
	if !ok || l.Burst() != cli.RPM { // the burst is the RPM itself: changed in the admin, recreate
		l = rate.NewLimiter(rate.Limit(float64(cli.RPM)/60), cli.RPM)
		s.limites[cli.ID] = l
	}
	s.limMu.Unlock()
	res := l.Reserve()
```
  The initialization in `Novo` becomes `limites: map[int64]*rate.Limiter{}`.
- [ ] `erro(...)` becomes one line: `responderJSON(w, status, map[string]any{"erro": map[string]string{"codigo": codigo, "mensagem": mensagem}})`.
- [ ] `roteador.go`: `slices.SortFunc(out, func(a, b EstadoDestino) int { return cmp.Compare(a.Destino, b.Destino) })`. Replace the `sort` import with `cmp` and `slices`.
- [ ] `gofmt -l . && go vet ./... && go test ./...` → clean and PASSES.
- [ ] Commit `refactor: cortes de stdlib apontados pela auditoria ponytail`.

### Task 6: Documentation
**Files:** `.claude/docs/backlog.md`, `.claude/docs/log.md`, `CLAUDE.md`, the architecture doc, `README.md`.
- [ ] Backlog:
  - Tick the 3 P0 items and M-5 as done.
  - Remove "Per-client concurrency limit" from P1 (already done in `3bc3677`).
  - Add to the debt table the lines `admin.go` (O(n) pruning), `auth.go` (single 32/key limit), `auth.go` (in-memory sessions) and `chat.go` (estimated usage without a flag).
  - Replace "push / PR #1" with the new PR.
- [ ] `log.md`: one line per task (date 10/10).
- [ ] `CLAUDE.md`:
  - In **Decisions**: "**XFF only behind a trusted proxy** (`K_SERVICE`/`TRUST_PROXY=1`): without a proxy the header belongs to the client and would give it a fresh bucket on every attempt."
  - In **Variables**: add `TRUST_PROXY`.
- [ ] Architecture doc (admin section) and `README.md` (variables): one line about `TRUST_PROXY` and the login limited on entry.
- [ ] Commit `docs: pendências de segurança do login resolvidas`.

### Task 7: Verification with `-race` + PR
- [ ] `docker run --rm -v "$PWD":/src -w /src golang:1.26 go test -race -count=2 ./...`. The Debian image already has gcc, so cgo works. Expected: every package `ok`.
- [ ] `go vet ./...` locally.
- [ ] Manual smoke with `go run ./cmd/comporta` and `curl`:
  - 6 POST `/admin/login` with a wrong token → the first 5 return 401 and the 6th returns 429.
  - Changing `X-Forwarded-For` still gives 429, because `TRUST_PROXY` isn't set.
- [ ] `code-reviewer` agent on the branch diff.
- [ ] `git push -u origin fix/login-seguranca`.
- [ ] `gh pr create --base main` with the title `fix: segurança do limite de login e revogação de chave`. Body: the 3 alerts, M-5, the cuts and the `-race` result, ending with `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
- [ ] **Don't merge.** That stays with Aurélio.

## End-to-end verification
`gofmt -l .` empty · `go vet ./...` · `-race` in Docker green (Task 7) · the curl smoke above · PR open against `main`.

## Interview question (end)
"Why trust the **last** `X-Forwarded-For` entry and not the first? And what changes with two proxies in front (load balancer + Cloud Run)?"
