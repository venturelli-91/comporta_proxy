# 01 — Config and cost

**Commit:** `6132d4c` · **Package:** `internal/gateway` (`config.go`, `custo.go`)

## What it does
Reads `comporta.json` (providers, model aliases, prices, peak windows, cache, timeouts), fills in the defaults
for anything left at zero, and computes the cost of a call in micro-dollars from the `usage` block, applying the
off-peak discount.

## Why this way (decisions)
- **Aliases** (`rapido` = fast, `forte` = strong, `migracao` = migration): the client never sees the real model name, so switching providers doesn't require changing any application.
- **Micro-dollars in `int64`**: `float` accumulates rounding error over sums of millions of calls. The float only exists while reading the config.
- **Sum before dividing**: on small calls, dividing token by token would zero out the cost.
- **A destination model without a price is refused at load time**: it would cost 0 and never consume the client's budget.
- **Defaults in one place** (`preencherPadroes`, called by `Carregar`): whoever uses the config gets it complete. Before, a missing `conexao_s` meant a dial without a timeout.

## How to test
`go test ./internal/gateway/ -v` (peak, off-peak, weekend, prefix cache, validation, defaults).

## Go vs TypeScript
- Struct tags (`json:"base_url"`) play the role of a Zod schema, but without validation: that's why `validar()` exists.
- `time.Time.UTC().Weekday()` replaces `date-fns`; there's no date dependency.

## Exercise
Add a second price window (for example, the small hours with a 75% discount) without breaking the existing tests.

## Interview question
"Why didn't you use `float64` for money, and where does the rounding happen?"
