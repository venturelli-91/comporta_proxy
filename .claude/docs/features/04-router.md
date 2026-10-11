# 04 — Router (semaphore, circuit breaker, retry, fallback)

**Package:** `internal/gateway` (`roteador.go`)

## What it does
`Chamar(ctx, client, alias, body)` takes an alias (`rapido`) all the way to a provider:
1. **The alias's semaphore** (a buffered channel the size of `concorrencia`). If no slot opens within 2 s → `ErrOcupado` (busy, 429).
2. For each destination in order: **circuit breaker** (`gobreaker/v2`, one per client and destination) → **retry** (3 attempts, feature 03) → `Cliente.Enviar`.
3. If the destination fails because of the provider (5xx, 429, network, breaker open, 401/402/403 of the gateway's account), it moves on to the next one. A client error (400/413/422) stops right away.
4. The semaphore slot is returned when the response body is closed, that is, only at the end of the stream.

`ChamarDestino` calls a specific destination and is reused by shadow mode. `Estado()` shows the breakers in the dashboard.

## Why this way (decisions)
- **Retry inside the breaker:** the breaker counts the final result. Three failed attempts count as one failure, not three.
- **The breaker opens with ≥ 5 calls and ≥ 50% failures:** a single early failure doesn't take the destination down.
- **A breaker per client and destination:** no client can provoke failures that open the destination for everyone else.
- **Client 4xx counts as success** and **cancellation is excluded**: neither says the provider is sick.
- **The semaphore slot is released on the body's `Close`** (`sync.OnceFunc`): a 2-minute stream holds the slot for the 2 minutes, which is what DeepSeek's limit measures.
- Ponytail: the "destinations + shadow" list became `ModeloCfg.todos()`, used by validation and by the router. Semaphore occupancy in `Estado()` waits for Prometheus metrics.

## How to test
`go test ./internal/gateway/ -run 'Fallback|Erro|Breaker|SemDestino|Semaforo' -v`

## Go vs TypeScript
- A buffered channel as a semaphore: `sem <- struct{}{}` acquires and `<-sem` releases. In Node it would be `p-limit`.
- `select` with three cases (slot, timeout, cancellation) has no direct `Promise` equivalent. It's the most "Go" feature in the file.
- Embedding `io.ReadCloser` in the `corpoComFim` struct and overriding only `Close` is composition, Go's "extends".

## Exercise
Implement the concurrency limit **per client** (on top of the per-alias limit), with a test showing that one client can't take every slot. (It now exists in `httpapi.ocupar`: compare your version with it.)

## Interview question
"Where did you put the retry relative to the circuit breaker, and what would change if it were the other way around?"
