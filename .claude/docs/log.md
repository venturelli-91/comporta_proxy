# Execution log

One line per feature or decision. Most recent at the bottom. Commit scopes before 10/11 are in Portuguese (history).

| Date | Feature / event | Commit | Notes |
|---|---|---|---|
| 10/09 | Initial spec (CLAUDE.md + architecture doc) | `5fd3867` | |
| 10/09 | Design spec and MVP plan v1 | `e7b18b3` | Choice: one OpenAI-compatible HTTP client + a fake on the same protocol |
| 10/09 | 01 Config and cost | `6132d4c` | Prices in `int64` micro-dollars; off-peak discount |
| 10/09 | Plan v2 + software house in `.claude/` | — | Branch `feat/mvp`; native execution; 8 software-house roles adapted |
| 10/09 | 02 | `feat(provedor)` | Single OpenAI-compatible client; fake on the same protocol; ponytail: ResponseController.Flush |
| 10/09 | 03 | `feat(resiliencia)` | Equal jitter (half as a floor); Retry-After wins; gives up if it doesn't fit the deadline |
| 10/09 | 04 | `feat(gateway)` | Retry inside the breaker; client 4xx doesn't open the breaker; semaphore slot released on body Close |
| 10/09 | 05 | `feat(cache)` | Byte-bounded LRU with container/list; lazy expiration; global mutex (ponytail) |
| 10/09 | 06 | `feat(lgpd)` | CPF/CNPJ check digits and Luhn; placeholder never collides with user text; fuzz 500k + 3.2M execs ok |
| 10/09 | 07 | `feat(armazenamento)` | rand.Text 128-bit key stored only as SHA-256; 500/1s batches drained on shutdown; p95 via OFFSET |
| 10/09 | 08 | `feat(httpapi)` | Overhead bench ~0.19 ms/req (loopback); singleflight 10→1; 499 and 504 tested; LGPD/singleflight mutants killed |
| 10/09 | 09 | `feat` | 23.5 MB distroless image; curl smoke ok; SIGTERM drains usage |
| 10/09 | 10 | `feat(web)` | Throughput bar as the centerpiece; TanStack v5 bug (undefined query data) caught in a screenshot; 24.3 MB image with the dashboard |
| 10/09 | 11 | `test(carga)` | 991 req/s p95 61.7 ms (60 ms fake) → ~1.7 ms overhead; cache avoids 99.9%; 50% chaos → 98.4% success |
| 10/09 | Ponytail audit (whole repo) | `refactor` | 1 cut: dead `nome` field/parameter in `provedor.Novo`; the rest lean, debt already marked with `ponytail:` |
| 10/09 | 12 Final docs | `docs` | README with numbers; CLAUDE.md (roadmap, decisions, pitch filled in); architecture doc (current state); communication updated |
| 10/09 | Final review fixes | `fix(httpapi)` | 7 important findings fixed with RED→GREEN tests; relay without a goroutine (watchdog); 104 tests; race 2x ok |
| 10/09 | Security: reused IDs | `fix(armazenamento)` | AUTOINCREMENT on clients and keys; old databases must be recreated (MVP without production data) |
| 10/09 | Security: concurrency limit per client | `fix(httpapi)` | 32 open requests per key (below the alias concurrency); above that, 429 concorrencia_excedida |
| 10/09 | Security: shared breaker | `fix(gateway)` | the request's total deadline doesn't open the breaker (only the watchdog counts); provider 401/402/403 fall back |
| 10/09 | Security: fail-open budget | `fix(gateway)` | config refuses destination/shadow models without a price; overshoot from in-flight requests bounded at 32 per client |
| 10/09 | Security: admin session | `fix(httpapi)` | cookie = random session (only the hash on the server), logout revokes, 5 attempts/min, Secure with X-Forwarded-Proto, compose requires ADMIN_TOKEN |
| 10/09 | Security: per-client breaker | `fix(gateway)` | breaker isolated per client+destination: nobody brings down the others' destination; an exceeded deadline counts again (a stuck provider opens the breaker for whoever suffers) |
| 10/09 | Security: login lockout | `fix(httpapi)` | login limit per IP (last X-Forwarded-For) and only failed attempts consume; an attacker can't lock the admin out |
| 10/09 | Session closed | `docs` | 3 alerts on the login limit (race, forgeable XFF, map reset) left for the next session: see the top of the backlog |
| 10/10 | Login security | `fix(httpapi)` | Allow() on entry (20 parallel → ≤5 compared); XFF only with K_SERVICE/TRUST_PROXY; pruning only full limiters |
| 10/10 | M-5 | `fix(armazenamento)` | revoking a key requires the client_id from the path |
| 10/10 | Ponytail cuts | `refactor` | cmp.Or, slices.SortFunc, no limiter struct: -18 lines |
| 10/10 | M-1 | `fix(httpapi)` | Recarregar under a mutex; deterministic test with a hook between read and swap (sync.Once was hiding the race) |
| 10/10 | M-6 | `fix(httpapi)` | lerResposta reads max+1: above 16 MiB becomes 502 and doesn't enter the cache |
| 10/10 | M-4 | `fix(httpapi)` | one LGPD restorer per delta field (content and reasoning_content) |
| 10/10 | M-3 | `fix(httpapi)` | Encerrar(ctx) cancels stuck shadows at the deadline; shutdown in 8 s (Cloud Run kills at 10 s) |
| 10/10 | Public demo | `feat` | Render free tier (no card) via render.yaml; DEMO_CHAVE recreates the demo client every time the disk is wiped; never DEEPSEEK_API_KEY there |
| 10/10 | Minor findings from the 10/10 review | `fix`/`test` | estimated cost for an oversized response; held-back reasoning flushed before content; parallel login requires exactly 5; XFF+LB documented |
| 10/10 | M-2, M-9, M-10 | `fix`/`test` | a lost usage batch goes to dropped + log; 499 checked in the table; a 401 on any screen goes back to login (QueryCache/MutationCache) |
| 10/10 | Refined dashboard | `feat(web)`/`feat(httpapi)` | light/dark/system theme; pointers; cards, badges, skeletons, icons; Playground before/after via X-Comporta-Enviado; checked in Chrome |
| 10/11 | Full review | `fix`/`refactor`/`docs` | JSON 404 for unknown /v1 and /admin routes; config defaults in one place (dial timeout); empty AdminToken refused in Novo; login only on 401; architecture §8 matched to the code (soft budget cap) |
| 10/11 | English, phase 1 | `docs`/`chore` | docs, `.claude/`, comments, UI and error messages in English; identifiers, packages and JSON contract left for phase 2 (backlog) |
