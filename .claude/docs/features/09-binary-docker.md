# 09 — Binary, Docker and compose

**Files:** `cmd/comporta/main.go`, `Dockerfile`, `docker-compose.yml`, `.dockerignore`

## What it does
- `main` reads the environment (`PORT`, `CONFIG_PATH`, `DB_PATH`, `ADMIN_TOKEN`, `DEEPSEEK_API_KEY`, `DEEPSEEK_BASE_URL`, `FALSO_*`) and wires config, providers, database, usage writer, cache, router and server.
- **Without an API key, the provider becomes the fake on loopback**, with configurable latency and errors. You can run, demo and measure without spending.
- **Graceful shutdown** on SIGINT/SIGTERM: stop accepting, wait for requests and shadows, drain the usage writer and only then close the database.
- Multi-stage image: build with `golang:1.27`, runtime `distroless/static:nonroot`, **23.5 MB**.

## Why this way (decisions)
- **No `WriteTimeout` on the `http.Server`:** it would cut long streams. Deadlines live in the relay's watchdog. `ReadHeaderTimeout` stays (Slowloris protection).
- **`CGO_ENABLED=0`:** only possible because of modernc.org/sqlite. A static binary runs on distroless.
- **`GOMEMLIMIT`:** the GC respects the container's limit instead of letting the process be OOM-killed.
- **Defers in LIFO order:** draining usage was registered after `banco.Fechar()`, so it runs before it. `go vet` caught a context leak on an error path, and the reorganization fixed it.
- **8 s shutdown deadline:** Cloud Run sends SIGKILL 10 s after SIGTERM; the remaining time goes to writing usage.
- Without `ADMIN_TOKEN`, the binary generates one and prints it to the terminal once (local convenience; in production it comes from Secret Manager).

## How to test
```bash
go run ./cmd/comporta            # uses the fake
docker compose up --build        # http://localhost:8080/healthz
```
Smoke on 10/09: `/healthz` and `/readyz` 200; client and key created through the admin; chat with and without streaming with the CPF restored; 401 without a key; `/metrics` counting 2 PII; no CPF in the logs; `docker compose stop` → "desligando" (shutting down).

## Go vs TypeScript
- `signal.NotifyContext` turns SIGTERM into a context cancellation, which in Node would be `process.on('SIGTERM')` + `server.close()`.
- `http.Server.Shutdown` waits for active connections. In Node, `server.close()` doesn't close keep-alives on its own.

## Exercise
Add `pprof` on an internal port (`127.0.0.1:6060`) enabled only when `PPROF=1`.

## Interview question
"How does your service behave when Cloud Run sends SIGTERM in the middle of a 2-minute stream?"
