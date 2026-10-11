---
name: go-gateway
description: Comporta's Go conventions and the per-feature checklist. Use before writing or reviewing any code in internal/ or cmd/ in this repository.
---

# Go at Comporta

The source of truth is `CLAUDE.md` (Go conventions, Security rules). This summary is for quick reference.

## Environment (Windows)
```powershell
$env:PATH="C:\Users\PC\sdk\go1.27.2\bin;$env:PATH"
go vet ./...; gofmt -l .
go test ./...
# -race needs cgo/gcc: run it in Docker
docker run --rm -v "${PWD}:/src" -v comporta-gomod:/go/pkg/mod -w /src golang:1.27 go test -race ./...
```

## Repository patterns
- Provider errors: `*provedor.ErroHTTP` with `Repetivel()` (retryable) and `Espera()` (wait). The retry (`resiliencia.Retry`) uses those two interfaces and doesn't depend on the provider package.
- Money in `int64` micro-dollars (`gateway.Config.Custo`).
- Network tests: `httptest.NewServer(&provedor.Falso{...})`. `Falso.Chamadas` counts the calls and is how the singleflight is proven.
- State read on every request (clients, keys): a snapshot in an `atomic.Pointer`, swapped whole.
- Config defaults are filled in `gateway.Carregar`; configs built by hand in tests set their timeouts explicitly.
- Client errors: `{"erro":{"codigo","mensagem"}}` with the status from the table in `CLAUDE.md`.
- Comments, log messages and error messages in English. Identifiers are still in Portuguese (phase 2).

## Idioms a TS dev tends to get wrong
- `for` with goroutines: since Go 1.22 each iteration has its own variable. No more `v := v`.
- A small interface defined in the package that consumes it, not the one that implements it.
- `defer resp.Body.Close()` only after checking `err`.
- `context` is Go's "AbortController": pass it along, don't store it in a struct.
- Maps aren't safe for concurrency: mutex, `sync.Map` or an immutable snapshot.

## Feature checklist
See `.claude/docs/checklist.md`.
