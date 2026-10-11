# 02 — Provider (OpenAI-compatible client + fake)

**Package:** `internal/provedor` (`cliente.go`, `falso.go`)

## What it does
- `Novo(baseURL, key, maxConns, dialTimeout)` creates the client; `Cliente.Enviar(ctx, body)` sends `POST /chat/completions` to any OpenAI-compatible API. A 2xx response comes back as is, so the relay can read it as a stream. Anything else becomes `*ErroHTTP{Status, RetryAfter, Mensagem}`.
- `Falso` (fake) is an `http.Handler` that mimics DeepSeek: it echoes the last message, sends `: keep-alive`, includes `usage` in the last chunk and ends with `[DONE]`. Latency, per-token speed and 503 rate are configurable.

## Why this way (decisions)
- **A single client:** DeepSeek and "openaicompat" differ only in the `base_url`. An interface with three implementations would be extra code.
- **The fake speaks real HTTP:** tests and k6 go through the same transport, connection pool and parser as the real path.
- **Its own `Transport`:** Go's default keeps only 2 idle connections per host, and under load that turns into TLS handshakes all the time.
- **No `http.Client.Timeout`:** it would kill long streams. Deadlines come from the `context`.
- **`Repetivel()` and `Espera()` as methods:** the retry package only sees the interfaces and doesn't import `provedor`.
- Ponytail applied: `http.NewResponseController(w).Flush()` instead of a manual `http.Flusher` assertion, and an anonymous struct instead of a single-use type.

## How to test
`go test ./internal/provedor/ -v`: non-streaming, streaming (keep-alive + usage + DONE), retryable 503, 429 with Retry-After, non-retryable 400, network down, cancelled context.

## Go vs TypeScript
- `context.Context` plays the role of `fetch`'s `AbortSignal`. Cancelling the context closes the connection to the provider.
- `errors.As(err, &e)` corresponds to `if (err instanceof ErroHTTP)`, except it works with errors wrapped by `%w`.
- `httptest.NewServer` starts a real server on a local port, which in Node would require `nock` or `msw`.

## Exercise
Support `Retry-After` in HTTP-date format (`http.ParseTime`), with a new case in the 429 test table.

## Interview question
"Why is Go's default HTTP transport bad for a high-volume proxy, and what did you tune?"
