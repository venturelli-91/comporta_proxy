# 09 — Binário, Docker e compose

**Arquivos:** `cmd/comporta/main.go`, `Dockerfile`, `docker-compose.yml`, `.dockerignore`

## O que faz
- `main` lê o ambiente (`PORT`, `CONFIG_PATH`, `DB_PATH`, `ADMIN_TOKEN`, `DEEPSEEK_API_KEY`, `DEEPSEEK_BASE_URL`, `FALSO_*`) e liga config, provedores, banco, gravador, cache, roteador e servidor.
- **Sem chave de API, o provedor vira o falso em loopback**, com latência e erro configuráveis. Dá para rodar, demonstrar e medir sem gastar.
- **Shutdown gracioso** com SIGINT/SIGTERM: para de aceitar, espera as requisições e os shadows, drena o gravador de uso e só então fecha o banco.
- Imagem multi-stage: build com `golang:1.27`, runtime `distroless/static:nonroot`, **23,5 MB**.

## Por que assim (decisões)
- **Sem `WriteTimeout` no `http.Server`:** ele cortaria streams longos. Os prazos ficam no watchdog do relay. O `ReadHeaderTimeout` continua (proteção contra Slowloris).
- **`CGO_ENABLED=0`:** só é possível por causa do modernc.org/sqlite. Binário estático roda no distroless.
- **`GOMEMLIMIT`:** o GC respeita o limite do container em vez de deixar o processo ser morto por OOM.
- **Defers em ordem LIFO:** a drenagem do uso foi registrada depois do `banco.Fechar()`, então roda antes dele. O `go vet` pegou um vazamento de contexto num caminho de erro, e a reorganização resolveu.
- Sem `ADMIN_TOKEN`, o binário gera um e mostra no terminal uma única vez (conveniência local; em produção vem do Secret Manager).

## Como testar
```bash
go run ./cmd/comporta            # usa o falso
docker compose up --build        # http://localhost:8080/healthz
```
Smoke de 09/10: `/healthz` e `/readyz` 200; cliente e chave criados pelo admin; chat com e sem stream com o CPF restaurado; 401 sem chave; `/metrics` contando 2 PII; nenhum CPF nos logs; `docker compose stop` → "desligando".

## Go vs TypeScript
- `signal.NotifyContext` transforma o SIGTERM em cancelamento de contexto, o que no Node seria `process.on('SIGTERM')` + `server.close()`.
- `http.Server.Shutdown` espera as conexões ativas. No Node, o `server.close()` não fecha keep-alives sozinho.

## Exercício
Acrescente um `pprof` numa porta interna (`127.0.0.1:6060`) ligado só quando `PPROF=1`.

## Pergunta de entrevista
"Como seu serviço se comporta quando o Cloud Run manda SIGTERM no meio de um stream de 2 minutos?"
