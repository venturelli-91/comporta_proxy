# Comporta — developer onboarding

## Prerequisites
Go 1.24+ (tested with 1.27.2), Docker, Node 20+ (for the dashboard).

On this Windows machine Go isn't on the PATH:
```powershell
$env:PATH="C:\Users\PC\sdk\go1.27.2\bin;$env:PATH"
```

## Run
```bash
go run ./cmd/comporta            # :8080, uses the fake provider without DEEPSEEK_API_KEY
cd web && npm install && npm run dev   # dashboard on :5173 proxying to :8080
docker compose up                # binary + embedded dashboard
```
Variables: `PORT`, `DEEPSEEK_API_KEY`, `DEEPSEEK_BASE_URL`, `ADMIN_TOKEN`, `DB_PATH`, `CONFIG_PATH` (see `.env.example`).

## Test
```bash
go vet ./... && gofmt -l .
go test ./...
# race detector (needs gcc on Windows, so it runs in the container):
docker run --rm -v "$PWD:/src" -v comporta-gomod:/go/pkg/mod -w /src golang:1.27 go test -race ./...
go test -run x -bench Overhead -benchmem ./internal/httpapi/
docker run --rm -i --network host grafana/k6 run - < carga/carga.js
```

## Contribute
1. Read `CLAUDE.md`, `.claude/softhouse.md` and `.claude/docs/architecture/backend.md`.
2. One feature = one commit, following `.claude/docs/checklist.md`.
3. Commits in English: `feat(scope): imperative description`. Comments, docs and UI text in English too.
4. Every diff goes through `code-reviewer` (with ponytail) and `qa-automation`.
