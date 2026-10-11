# 03 — Retry with backoff and jitter

**Package:** `internal/resiliencia` (`retry.go`)

## What it does
`Retry(ctx, attempts, base, fn)` calls `fn` up to N times. It only retries errors that report `Repetivel() == true`
(retryable: 429, 5xx, network). The wait doubles on each attempt: half fixed, half random. A provider's
`Retry-After` replaces the calculation. If the wait doesn't fit in the context's deadline, it gives up right away.

## Why this way (decisions)
- **Interfaces in the consumer:** `repetivel` and `comEspera` are declared in the `resiliencia` package. `provedor.ErroHTTP` satisfies them without knowing they exist (structural typing, like in TypeScript).
- **"Equal jitter" instead of full jitter:** the floor of half the wait prevents near-immediate retries, and the test stays deterministic.
- **Give up before blowing the deadline:** sleeping until the context dies only wastes a goroutine and hides the real error.
- **Retry only before the first byte:** the caller (the router) guarantees this. `Retry` knows nothing about streams.

## How to test
`go test ./internal/resiliencia/ -v`: success on the 1st and 3rd attempts, attempts exhausted, non-retryable error, plain error, Retry-After, context with a short deadline.

## Go vs TypeScript
- In Go a struct implements an interface without declaring `implements`. It's the same structural duck typing as TS, but checked at compile time.
- `select` with `ctx.Done()` and a timer is the equivalent of `Promise.race([sleep(ms), abortPromise])`.

## Exercise
Add a maximum wait (for example, 5 s) so that `base << i` doesn't grow without bound, with a case in the table.

## Interview question
"Why jitter? What happens with a thousand clients retrying at exactly the same interval after an outage?"
