# Comporta MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** gateway de LLM em Go que sustenta o pitch da entrevista: chaves por cliente, limite, orçamento, cache + singleflight, resiliência, LGPD, shadow, painel e números de k6.

**Architecture:** um único cliente HTTP compatível com OpenAI fala com DeepSeek ou com o provedor falso (que é um `http.Handler` do mesmo protocolo). O handler de chat encadeia autenticação → admissão → LGPD → cache/singleflight → roteador (semáforo, breaker, retry, fallback) → relay. Uso vai por canal para um gravador em lote no SQLite.

**Tech Stack:** Go 1.27 (stdlib `net/http`, `log/slog`, `encoding/json`, `container/list`, `embed`), `golang.org/x/time/rate`, `golang.org/x/sync/singleflight`, `github.com/sony/gobreaker/v2`, `modernc.org/sqlite`; React + Vite + TS + TanStack Query + Recharts + Tailwind; k6 via Docker.

**Spec:** `docs/superpowers/specs/2026-10-09-comporta-design.md`

> Execução escolhida pelo usuário: **Native** (eu implemento tudo nesta sessão). O código vive no
> repositório; cada tarefa abaixo registra contrato, testes e commit, sem duplicar o código aqui.

## Global Constraints

- Go 1.24+ (instalado: 1.27.2). Dependências só as da lista acima; qualquer outra precisa de aprovação.
- `go vet ./...` limpo; `go test -race ./...` passando (o race roda no Docker `golang:1.27`, porque no Windows exige gcc).
- Erros sempre `{"erro": {"codigo": "...", "mensagem": "..."}}`.
- `DEEPSEEK_API_KEY` nunca em log, front ou repositório. Chaves de cliente só como hash SHA-256. Comparação de tokens com `crypto/subtle`.
- Conteúdo de mensagens nunca é logado nem gravado. O mapa de PII só existe na memória da requisição.
- Mascaramento antes do cache. Dinheiro em micro-dólares `int64`.
- Commits: Conventional Commits em português, um por tarefa, com `Co-Authored-By`.
- Comentários em português explicando o porquê. Simplificações deliberadas marcadas com `ponytail:`.

## Review Focus

1. Cliente desconecta no meio do stream → a chamada ao provedor é cancelada e o uso é registrado com status 499.
2. Provedor manda só `: keep-alive` por muito tempo → 504 do watchdog, não conexão pendurada.
3. Texto que já contém algo como `[CPF_1]` → restaurar não pode trocar o literal do usuário (fuzz cobre).
4. Chave revogada continua funcionando por cache de snapshot → recarregar snapshot a cada mutação do admin (teste de revogação).
5. Canal de uso lotado → a API não trava; descarta e conta (teste com canal de capacidade 1).

---

### Task 1: Esqueleto do módulo, config e custo

**Files:** `go.mod`, `.gitignore`, `.gitattributes`, `.env.example`, `comporta.json`, `internal/gateway/config.go`, `internal/gateway/custo.go`, `internal/gateway/custo_test.go`

**Produces:**
- `gateway.Carregar(caminho string) (*Config, error)`
- `Config{Provedores map[string]ProvedorCfg; Modelos map[string]ModeloCfg; Precos map[string]Preco; Pico PicoCfg}`
- `ModeloCfg{Destinos []Destino; Shadow *Destino; Concorrencia int}`; `Destino{Provedor, Modelo string}`
- `Uso{PromptTokens, CompletionTokens, CacheHitTokens, CacheMissTokens, ReasoningTokens int64}`
- `(*Config).Custo(modelo string, u Uso, t time.Time) int64` (micro-dólares)

- [ ] Teste table-driven de custo: pico, fora do pico (desconto), fim de semana, cache de prefixo, modelo sem preço → 0.
- [ ] Implementar `Carregar` (valida alias sem destino e destino com provedor inexistente) e `Custo`.
- [ ] `go test ./internal/gateway/` passa. Commit `feat(gateway): config em JSON e cálculo de custo com horário de pico`.

### Task 2: Provedor (cliente HTTP + falso)

**Files:** `internal/provedor/cliente.go`, `internal/provedor/falso.go`, `internal/provedor/provedor_test.go`

**Produces:**
- `provedor.Novo(nome, baseURL, chave string, maxConexoes int) *Cliente`
- `(*Cliente).Enviar(ctx, corpo []byte) (*http.Response, error)` — 2xx devolve a resposta; senão `*ErroHTTP{Status int; RetryAfter time.Duration; Mensagem string}`
- `provedor.Falso{Latencia, PorToken time.Duration; TaxaErro float64}` implementa `http.Handler` em `POST /chat/completions`: eco da última mensagem, keep-alive, `usage` no último chunk, `[DONE]`.

- [ ] Testes contra `httptest.NewServer(&Falso{...})`: sem stream, stream (keep-alive + usage + DONE), 503 com `TaxaErro: 1`, 429 com `Retry-After`, cancelamento por contexto.
- [ ] Implementar. Commit `feat(provedor): cliente compatível com OpenAI e provedor falso`.

### Task 3: Retry com backoff e jitter

**Files:** `internal/resiliencia/retry.go`, `internal/resiliencia/retry_test.go`

**Produces:** `resiliencia.Retry(ctx, tentativas int, base time.Duration, fn func() error) error` — repete se `Repetivel(err)`; respeita `RetryAfter() time.Duration` do erro; para no fim do prazo do contexto.

- [ ] Table-driven: sucesso na 1ª, sucesso na 3ª, erro não repetível sai na 1ª, `Retry-After` respeitado, contexto cancelado.
- [ ] Commit `feat(resiliencia): retry com backoff exponencial, jitter e Retry-After`.

### Task 4: Roteador (semáforo, breaker, fallback)

**Files:** `internal/gateway/roteador.go`, `internal/gateway/roteador_test.go`

**Consumes:** Tasks 1–3. **Produces:**
- `gateway.NovoRoteador(cfg *Config, clientes map[string]*provedor.Cliente) *Roteador`
- `(*Roteador).Chamar(ctx, alias string, corpo map[string]json.RawMessage) (*http.Response, Destino, error)` — troca `model` pelo do destino.
- `(*Roteador).Estado() []EstadoDestino` (breaker e ocupação do semáforo)
- Erros: `ErrSemDestino`, `ErrOcupado` (semáforo), `*provedor.ErroHTTP` (4xx do cliente não troca de destino).

- [ ] Testes: fallback quando o principal dá 503; 400 do provedor não faz fallback; breaker abre após falhas e pula o destino; semáforo nunca passa do limite (concorrente, `-race`).
- [ ] Commit `feat(gateway): roteador com semáforo, circuit breaker, retry e fallback`.

### Task 5: Cache LRU com TTL

**Files:** `internal/cache/cache.go`, `internal/cache/cache_test.go`

**Produces:** `cache.Novo(maxBytes int64, ttl time.Duration) *Cache`; `Get(chave) (Entrada, bool)`; `Set(chave, Entrada)`; `Entrada{Corpo []byte; Stream bool; Custo int64}`.

- [ ] Testes: hit, expiração por TTL, despejo LRU por bytes, acesso concorrente com `-race`.
- [ ] Commit `feat(cache): LRU com TTL e limite de memória`.

### Task 6: LGPD

**Files:** `internal/lgpd/lgpd.go`, `internal/lgpd/lgpd_test.go`

**Produces:** `lgpd.NovoMapa() *Mapa`; `(*Mapa).Mascarar(s string) string`; `(*Mapa).Restaurar(s string) string`; `(*Mapa).Len() int`; `(*Mapa).NovoStream() *Stream`; `(*Stream).Escrever(s string) string`; `(*Stream).Finalizar() string`.

- [ ] Table-driven: CPF/CNPJ com e sem pontuação e DV inválido, e-mail, celular/fixo com e sem +55, cartão com Luhn válido/inválido, mesmo valor → mesmo marcador.
- [ ] Fuzz: `Restaurar(Mascarar(s)) == s`; stream com cortes arbitrários devolve o mesmo que `Restaurar`.
- [ ] Commit `feat(lgpd): mascaramento de CPF, CNPJ, e-mail, telefone e cartão com restauração em stream`.

### Task 7: Armazenamento

**Files:** `internal/armazenamento/banco.go`, `internal/armazenamento/uso.go`, `internal/armazenamento/banco_test.go`

**Produces:**
- `armazenamento.Abrir(caminho string) (*Banco, error)` (WAL, migração idempotente)
- `Cliente{ID int64; Nome string; RPM int; OrcamentoMicro int64; Cache, LGPD bool; Modelos []string}`
- `CriarCliente`, `AtualizarCliente`, `RemoverCliente`, `ListarClientes`, `CriarChave(clienteID) (texto string, Chave, error)`, `RevogarChave`, `ListarChaves`, `ChavesAtivas() (map[string]int64, error)` (hash → cliente)
- `Evento{...}`; `NovoGravador(b, capacidade int) *Gravador`; `(*Gravador).Registrar(Evento) bool`; `Rodar(ctx)`; `Descartados() int64`
- `(*Banco).Relatorio(ctx, de, ate time.Time, cliente int64) (Relatorio, error)`; `GastoDoMes(ctx, t) (map[int64]int64, error)`

- [ ] Testes com SQLite temporário: CRUD, chave só por hash, revogação, gravador em lote e descarte com canal cheio, relatório com p95.
- [ ] Commit `feat(armazenamento): SQLite com clientes, chaves por hash e uso em lote`.

### Task 8: API HTTP (chat, admin, métricas)

**Files:** `internal/httpapi/servidor.go`, `auth.go`, `chat.go`, `relay.go`, `admin.go`, `metricas.go`, `chat_test.go`, `bench_test.go`; `web/embed.go`

**Produces:** `httpapi.Novo(Deps) (*Servidor, error)`; `(*Servidor).Handler() http.Handler`; `(*Servidor).Recarregar(ctx) error`.

- [ ] Testes de integração (`httptest` + falso): 401, 429 com `Retry-After`, 402, 403, sem stream, stream, cache hit, singleflight N→1 (contador no falso), LGPD ida e volta (o falso ecoa o marcador; o cliente recebe o CPF), fallback com 503, revogação de chave, shadow registra evento.
- [ ] `BenchmarkOverhead` com `-benchmem`.
- [ ] Commit `feat(httpapi): chat compatível com OpenAI, admin e métricas`.

### Task 9: main + Docker

**Files:** `cmd/comporta/main.go`, `Dockerfile`, `docker-compose.yml`

- [ ] Sobe falso em loopback quando não há `DEEPSEEK_API_KEY`; shutdown gracioso drena o gravador.
- [ ] Smoke: `curl` sem stream e com stream contra `go run`.
- [ ] Commit `feat: binário comporta com shutdown gracioso e imagem distroless`.

### Task 10: Painel React

**Files:** `web/` (Vite), `web/src/*`

- [ ] Visão geral (cartões + gráfico por minuto + tabela por cliente), Clientes e chaves, Shadow, Playground com stream.
- [ ] `npm run build` gera `web/dist` embutido no binário.
- [ ] Commit `feat(web): painel com visão geral, clientes, shadow e playground`.

### Task 11: Carga e números

**Files:** `carga/carga.js`

- [ ] k6 via Docker: cenários `sem_cache`, `com_cache`, `caos` (50% de 503). Números no README.
- [ ] Commit `test(carga): cenários k6 contra o provedor falso`.

### Task 12: Documentação final

**Files:** `README.md`, `CHANGELOG.md`, `docs/BACKLOG.md`, `docs/ARQUITETURA.md`, `CLAUDE.md`

- [ ] README de portfólio, CHANGELOG por commit, backlog com checklist, decisões novas no CLAUDE.md e na arquitetura.
- [ ] Commit `docs: README, changelog, backlog e decisões atualizadas`.
