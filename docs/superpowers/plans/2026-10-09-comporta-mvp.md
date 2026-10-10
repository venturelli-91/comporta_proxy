# Comporta MVP — Plano de execução (v2: softhouse + .claude + ponytail)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans (execução Native escolhida pelo usuário). Checkbox `- [ ]` para acompanhar.

## Context

O `CLAUDE.md` define a Comporta (gateway de LLM em Go) para a entrevista de 14/10/2026. A spec
(`docs/superpowers/specs/2026-10-09-comporta-design.md`) e o plano v1 (`docs/superpowers/plans/2026-10-09-comporta-mvp.md`)
já foram aprovados numa sessão anterior. A Task 1 está commitada (`6132d4c`). O código da Task 2
(`internal/provedor/`) existe mas não está no git. Agora o usuário pede: implementar todo o plano em modo
automático, **um commit por feature**, seguindo as regras da softhouse dele (`venturelli-house/`, excluída do
git via `.git/info/exclude`), com tudo documentado em `.claude/` e o ponytail auditando simplificações.

**Decisões do usuário (09/10):** branch `feat/mvp` + push + PR no fim (o merge fica com ele); levar para o
projeto só os papéis da softhouse que servem aqui; execução Native com review independente no fim.

**Decisões minhas, para ele confirmar:**
- Commits em **Conventional Commits em português** + `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`. O CLAUDE.md do projeto ("mensagens claras em português") e os commits que já existem valem mais que a regra global de commits em inglês.
- O "modo aprendizado" do CLAUDE.md vira documentação: cada feature ganha em `.claude/docs/features/` um exercício sugerido, uma comparação com TS/Node e uma pergunta de entrevista. O usuário pediu modo automático, então eu escrevo o código.
- O plano v1 continua no formato de contrato, sem repetir o código, como já tinha sido combinado. Este v2 substitui o v1 no mesmo caminho dentro do repo (Task 0).
- Pela regra global, aviso o usuário quando o contexto chegar a 40%.

## Global Constraints (valem para todas as tarefas)

- Go 1.27.2 em `C:\Users\PC\sdk\go1.27.2\bin`, que não está no PATH. Em todo comando: `$env:PATH="C:\Users\PC\sdk\go1.27.2\bin;$env:PATH"`.
- O race detector precisa de gcc no Windows, então roda no Docker: `docker run --rm -v "${PWD}:/src" -v comporta-gomod:/go/pkg/mod -w /src golang:1.27 go test -race ./...`
- Dependências permitidas, e só elas: `golang.org/x/time/rate`, `golang.org/x/sync`, `github.com/sony/gobreaker/v2`, `modernc.org/sqlite`. Front: Vite, React, TS, TanStack Query, Recharts, Tailwind.
- Erros no formato `{"erro":{"codigo","mensagem"}}`. Dinheiro em micro-dólares `int64`. Mascarar antes de cachear. Conteúdo das mensagens nunca vai para log ou banco. Chaves guardadas como SHA-256 e comparadas com `crypto/subtle`.
- Comentários em português explicando o porquê. Simplificações deliberadas marcadas com `// ponytail:`, dizendo o limite e o caminho de upgrade.
- Matriz de autonomia da softhouse: o merge na main, o deploy e qualquer dependência fora da lista acima precisam de aprovação do usuário.

## Review Focus (cada item ganha teste na tarefa dona)
1. O cliente desconecta no meio do stream: a chamada ao provedor é cancelada e o uso é registrado com status 499 (Task 8).
2. O provedor manda só `: keep-alive`: o watchdog responde 504 (Task 8).
3. O texto já contém o literal `[CPF_1]`: restaurar não troca o literal do usuário (fuzz da Task 6).
4. Chave revogada deixa de funcionar na hora, porque o snapshot é recarregado a cada mutação do admin (Task 8).
5. Canal de uso cheio: a API não trava, descarta o evento e conta (Task 7).

## Ciclo de cada feature (Definition of Done, repetido em `.claude/docs/checklist.md`)
1. Reler a seção correspondente de `docs/ARQUITETURA.md` e da spec.
2. TDD: teste falhando, código mínimo, teste passando.
3. `go vet ./...` e `gofmt -l .` sem saída. Testes com `-race` passando no Docker.
4. `ponytail:ponytail-review` sobre o diff da feature. Aplicar os cortes e marcar o que fica com `ponytail:`.
5. Criar `.claude/docs/features/NN-<nome>.md` (o quê, porquê, decisões, como testar, Go vs TS, exercício, pergunta de entrevista). Acrescentar uma linha em `.claude/docs/log.md` e marcar o item em `.claude/docs/backlog.md`.
6. Um commit com o código e a documentação daquela feature.

---

### Task 0: Branch, softhouse e estrutura `.claude/`
- [ ] `git switch -c feat/mvp`
- [ ] Criar a estrutura:
```
.claude/
  README.md                 índice da pasta e de como usar os agentes e comandos
  softhouse.md              regras da venturelli-house adaptadas (princípios, fluxo, roteamento, matriz de autonomia, ponytail em todo PR)
  agents/                   tech-lead, dev-backend, dev-frontend, code-reviewer (com ponytail-review), qa-automation,
                            seguranca (LGPD/chaves), performance (k6/latência), devops-sre (Docker/Cloud Run)
                            -> adaptados de venturelli-house/.claude/agents/*.md para Go + React + este repo
  commands/                 ticket.md, daily-report.md, feature.md (roda o ciclo de feature acima)
  skills/
    go-gateway/SKILL.md     convenções Go do projeto + checklist de feature (resume o CLAUDE.md, não duplica)
    entrevista/SKILL.md     gera perguntas e respostas de entrevista a partir de .claude/docs/features
  docs/
    log.md                  diário: data, feature, commit, números, decisões
    backlog.md              roadmap do CLAUDE.md como checklist (MVP / se sobrar tempo / depois)
    checklist.md            DoD por feature, checklist pré-PR e pré-entrevista
    features/               um arquivo por feature (criado em cada task)
    arquitetura/backend.md  mapa de pacotes, caminho da requisição, interfaces (aponta para ARQUITETURA.md e a spec)
    arquitetura/frontend.md estrutura do painel, rotas, estado (TanStack Query), embed no binário
    design/painel.md        telas, tokens, componentes, estados vazio/erro
    comunicacao/stakeholders.md  pitch e status sem jargão (não técnicos)
    comunicacao/tech-leads.md    decisões, trade-offs, riscos
    comunicacao/devs.md          onboarding: rodar, testar, contribuir
```
- [ ] Copiar este plano para `docs/superpowers/plans/2026-10-09-comporta-mvp.md`, substituindo o v1.
- [ ] Commit `docs(claude): softhouse, agentes, skills e documentação de processo`.

### Task 1: Config e custo ✅ (`6132d4c`)
Só criar `.claude/docs/features/01-config-custo.md` e a linha no log. Entra no commit da Task 0.

### Task 2: Provedor (cliente HTTP compatível com OpenAI + falso)
**Arquivos:** `internal/provedor/{cliente,falso,provedor_test}.go` (já existem, sem commit). Apagar `internal/provedor/.impeccable/` e colocar `.impeccable/` no `.gitignore`.
**Produz:** `provedor.Novo(nome, baseURL, chave string, maxConexoes int, timeoutConexao time.Duration) *Cliente`; `(*Cliente).Enviar(ctx, corpo []byte) (*http.Response, error)`; `*ErroHTTP{Status, RetryAfter, Mensagem}` com `Repetivel()` e `Espera()`; `Falso{Latencia, PorToken, TaxaErro, Chamadas atomic.Int64}`, que implementa `http.Handler`.
- [ ] Rodar os testes que já existem (sem stream, stream, 503, 429 com Retry-After, 400, rede fora, contexto cancelado).
- [ ] Ciclo de feature. Commit `feat(provedor): cliente compatível com OpenAI e provedor falso`.

### Task 3: Retry com backoff e jitter
**Arquivos:** `internal/resiliencia/retry.go`, `retry_test.go`.
**Produz:** `resiliencia.Retry(ctx, tentativas int, base time.Duration, fn func() error) error`. Repete quando o erro implementa `Repetivel() bool` e devolve true; usa `Espera() time.Duration` quando for maior que zero; desiste se o prazo do contexto não comportar a espera.
- [ ] Table-driven: sucesso na 1ª tentativa; sucesso na 3ª; erro não repetível sai na 1ª; Retry-After respeitado; contexto cancelado durante a espera.
- [ ] Commit `feat(resiliencia): retry com backoff exponencial, jitter e Retry-After`.

### Task 4: Roteador (semáforo, breaker, fallback)
**Arquivos:** `internal/gateway/roteador.go`, `roteador_test.go`. Dependência nova: `github.com/sony/gobreaker/v2`.
**Consome:** `Config`/`ModeloCfg`/`Destino` (Task 1), `provedor.Cliente` e `ErroHTTP` (Task 2), `resiliencia.Retry` (Task 3).
**Produz:** `NovoRoteador(cfg *Config, clientes map[string]*provedor.Cliente) *Roteador`; `(*Roteador).Chamar(ctx, alias string, corpo map[string]json.RawMessage) (*http.Response, Destino, error)`; `Estado() []EstadoDestino`; `ErrSemDestino`, `ErrOcupado`.
- [ ] Testes: o principal devolve 503 e cai no fallback; 400 não faz fallback; o breaker abre e pula o destino; com o semáforo, a concorrência nunca passa do limite (teste com `-race`).
- [ ] Commit `feat(gateway): roteador com semáforo, circuit breaker, retry e fallback`.

### Task 5: Cache LRU com TTL
**Arquivos:** `internal/cache/cache.go`, `cache_test.go`, com `container/list` e mutex.
**Produz:** `cache.Novo(maxBytes int64, ttl time.Duration) *Cache`; `Get(string) (Entrada, bool)`; `Set(string, Entrada)`; `Entrada{Corpo []byte; Stream bool; Custo int64}`.
- [ ] Testes: hit, expiração pelo TTL, despejo LRU por bytes, acesso concorrente com `-race`.
- [ ] Commit `feat(cache): LRU com TTL e limite de memória`.

### Task 6: LGPD
**Arquivos:** `internal/lgpd/lgpd.go`, `lgpd_test.go`.
**Produz:** `NovoMapa() *Mapa`; `Mascarar`, `Restaurar`, `Len`; `NovoStream() *Stream`; `(*Stream).Escrever(string) string`; `Finalizar() string`.
- [ ] Table-driven: CPF/CNPJ com e sem pontuação, e com dígito verificador inválido; e-mail; celular e fixo, com e sem +55; cartão com Luhn válido e inválido; o mesmo valor sempre recebe o mesmo marcador.
- [ ] Fuzz: `Restaurar(Mascarar(s)) == s`, inclusive quando `s` já contém um marcador literal. No stream com cortes arbitrários, o resultado é igual ao de `Restaurar`.
- [ ] Commit `feat(lgpd): mascaramento de CPF, CNPJ, e-mail, telefone e cartão com restauração em stream`.

### Task 7: Armazenamento
**Arquivos:** `internal/armazenamento/{banco,uso,banco_test}.go`. Dependência nova: `modernc.org/sqlite`.
**Produz:** os contratos da Task 7 do plano v1 (`Abrir`, CRUD de `Cliente`, `CriarChave` → texto `ck_...` mostrado uma única vez, `ChavesAtivas` como mapa hash → cliente, `Gravador` em lote com `Registrar(Evento) bool` e `Descartados()`, `Relatorio` com p95, `GastoDoMes`).
- [ ] Testes com banco temporário: CRUD; chave guardada só como hash; revogação; gravação em lote; descarte com canal de capacidade 1; relatório com p95.
- [ ] Commit `feat(armazenamento): SQLite com clientes, chaves por hash e uso em lote`.

### Task 8: API HTTP
**Arquivos:** `internal/httpapi/{servidor,auth,chat,relay,admin,metricas,chat_test,bench_test}.go`, `web/embed.go` (com `web/dist/.gitkeep`). Dependências novas: `x/time/rate`, `x/sync/singleflight`.
**Produz:** `httpapi.Novo(Deps) (*Servidor, error)`; `Handler() http.Handler`; `Recarregar(ctx) error`.
- [ ] Testes de integração com `httptest` e o falso: 401; 429 com Retry-After; 402; 403; resposta sem stream; stream; cache hit; singleflight (N pedidos → 1 chamada, contada por `Falso.Chamadas`); LGPD ida e volta; fallback com 503; revogação; shadow; desconexão com 499; watchdog com 504.
- [ ] `BenchmarkOverhead` com `-benchmem`.
- [ ] Commit `feat(httpapi): chat compatível com OpenAI, admin e métricas`.

### Task 9: main + Docker
**Arquivos:** `cmd/comporta/main.go`, `Dockerfile` (multi-stage, distroless), `docker-compose.yml`.
- [ ] Sem `DEEPSEEK_API_KEY`, sobe o falso em loopback. O shutdown gracioso drena o gravador.
- [ ] Smoke: `go run ./cmd/comporta` + `curl`, com e sem stream. `docker compose up` responde em `/healthz`.
- [ ] Commit `feat: binário comporta com shutdown gracioso e imagem distroless`.

### Task 10: Painel React
**Arquivos:** `web/` (Vite + React + TS + TanStack Query + Recharts + Tailwind). Design conforme `.claude/docs/design/painel.md`, usando a skill `frontend-design`.
- [ ] Telas: Visão geral (cartões, gráfico por minuto, tabela por cliente), Clientes e chaves, Shadow, Playground com stream.
- [ ] `npm run build` gera `web/dist`, embutido no binário. Smoke no navegador com o binário.
- [ ] Commit `feat(web): painel com visão geral, clientes, shadow e playground`.

### Task 11: Carga
**Arquivos:** `carga/carga.js`. Roda o k6 via Docker (`grafana/k6`) contra o binário com o falso.
- [ ] Cenários `sem_cache`, `com_cache` e `caos` (50% de 503). Números em `.claude/docs/log.md`.
- [ ] Commit `test(carga): cenários k6 contra o provedor falso`.

### Task 12: Auditoria ponytail, docs finais e PR
- [ ] `ponytail:ponytail-audit` no repo inteiro. Aplicar o que for seguro, com testes passando. Commit `refactor: simplificações apontadas pela auditoria ponytail`. Se não houver cortes, só registrar no log.
- [ ] `ponytail:ponytail-debt` → `.claude/docs/backlog.md` (seção "Dívida deliberada").
- [ ] README de portfólio (arquitetura, decisões, números do k6, limitações LGPD); atualizar o roadmap e as decisões em `CLAUDE.md` e `docs/ARQUITETURA.md`; finalizar os documentos de comunicação. Commit `docs: README, decisões e comunicação finais`.
- [ ] Review independente do branch inteiro (`superpowers:requesting-code-review`). Corrigir o que bloquear.
- [ ] `git push -u origin feat/mvp` + `gh pr create` (corpo com resumo, evidências e a linha 🤖 de atribuição). **Não fazer merge.**

## Verificação ponta a ponta
- `go vet ./...` e `gofmt -l .` sem saída; `go test -race ./...` verde no Docker.
- `go run ./cmd/comporta`: `curl -H "Authorization: Bearer ck_..." localhost:8080/v1/chat/completions`, com e sem `stream:true`. Um CPF válido no prompt volta restaurado e chega mascarado no falso.
- O painel abre em `localhost:8080`, mostra o uso e o playground faz stream.
- k6: os três cenários rodam e os números ficam registrados.
- PR aberto no GitHub com todos os commits por feature.
