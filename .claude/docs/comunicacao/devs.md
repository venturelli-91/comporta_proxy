# Comporta — onboarding de devs

## Pré-requisitos
Go 1.24+ (testado com 1.27.2), Docker, Node 20+ (para o painel).

No Windows desta máquina o Go não está no PATH:
```powershell
$env:PATH="C:\Users\PC\sdk\go1.27.2\bin;$env:PATH"
```

## Rodar
```bash
go run ./cmd/comporta            # :8080, usa o provedor falso sem DEEPSEEK_API_KEY
cd web && npm install && npm run dev   # painel em :5173 com proxy para :8080
docker compose up                # binário + painel embutido
```
Variáveis: `PORT`, `DEEPSEEK_API_KEY`, `DEEPSEEK_BASE_URL`, `ADMIN_TOKEN`, `DB_PATH`, `CONFIG_PATH` (ver `.env.example`).

## Testar
```bash
go vet ./... && gofmt -l .
go test ./...
# race detector (precisa de gcc no Windows, então roda no container):
docker run --rm -v "$PWD:/src" -v comporta-gomod:/go/pkg/mod -w /src golang:1.27 go test -race ./...
go test -run x -bench Overhead -benchmem ./internal/httpapi/
docker run --rm -i --network host grafana/k6 run - < carga/carga.js
```

## Contribuir
1. Leia `CLAUDE.md`, `.claude/softhouse.md` e `.claude/docs/arquitetura/backend.md`.
2. Uma feature = um commit, seguindo `.claude/docs/checklist.md`.
3. Commits em português: `feat(escopo): descrição no imperativo`.
4. Todo diff passa pelo `code-reviewer` (com ponytail) e pelo `qa-automation`.
