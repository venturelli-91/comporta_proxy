# 07 — Storage (SQLite)

**Package:** `internal/armazenamento` (`banco.go`, `uso.go`)

## What it does
- Clients (RPM, monthly budget, cache, LGPD, allowed aliases), with CRUD.
- `ck_...` keys: generated with `crypto/rand.Text()` (128 bits), shown **once** and stored only as SHA-256.
- `ChavesAtivas()` (active keys) returns hash → client, for the in-memory authentication snapshot.
- `Gravador` (writer): a buffered channel + a goroutine that writes in batches (500 events or 1 s). Channel full = drop and count.
- `Relatorio` (report): totals, p95, per client, per model, per-minute series and shadow comparison. `GastoDoMes` (month's spend) feeds the budget.

## Why this way (decisions)
- **modernc.org/sqlite:** no cgo, so the binary compiles anywhere and the distroless image works.
- **WAL + busy_timeout:** the dashboard reads while the writer writes.
- **Unsalted SHA-256:** the key has 128 random bits, with no dictionary to attack. Bcrypt is for human passwords.
- **Batched writes, off the request path:** one INSERT per request would add latency and contend for the write lock.
- **Shutdown drains the channel:** no event already accepted is lost.
- **AUTOINCREMENT:** without it SQLite reuses the highest deleted ID, and a new client would inherit the deleted one's history, spend and limiter.
- **Nearest-rank p95** with `ORDER BY ... LIMIT 1 OFFSET`, because SQLite has no percentile function.
- **Shadow outside the totals and the budget:** the shadow call wasn't answered to anyone and is the admin's decision, not the client's.
- Ponytail: no `Repository` interface (a single implementation). The report scans the raw table; `uso_minuto` comes in when it shows up in a profile.

## How to test
`go test ./internal/armazenamento/ -v`: CRUD, key only as a hash, revocation, batch drained on shutdown, full channel without blocking, report with p95, client filter, shadow and month's spend.

## Go vs TypeScript
- `database/sql` is the generic interface, and the driver comes in through `import _` (registration side effect). In Node, each driver has its own API.
- `select { case ch <- e: ... default: ... }` is a non-blocking send, with no direct Promise equivalent.
- `defer tx.Rollback()` after `Commit` does nothing. It's the "roll back if we didn't reach the commit" pattern.

## Exercise
Implement the `uso_minuto` table, aggregated in the same batch, and make the report's `Serie` read from it. Compare the time with 1 million rows.

## Interview question
"What happens to the usage records if the database gets slow, and why did you accept losing data there?"
