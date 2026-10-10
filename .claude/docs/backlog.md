# Backlog

Prioridades da softhouse: P0 (prazo da entrevista), P1 (semana), P2 (melhoria).

## MVP — até 13/10 (P0)
- [x] 01 Config em JSON e custo com horário de pico
- [x] 02 Provedor: cliente compatível com OpenAI + provedor falso
- [x] 03 Retry com backoff, jitter e Retry-After
- [x] 04 Roteador: semáforo, circuit breaker, fallback
- [x] 05 Cache LRU com TTL
- [x] 06 LGPD: mascaramento + restauração (inclusive em stream)
- [x] 07 Armazenamento: clientes, chaves por hash, uso em lote
- [x] 08 API HTTP: chat (com e sem stream), auth, rate limit, orçamento, cache + singleflight, shadow, admin, métricas
- [x] 09 Binário + Docker + compose
- [x] 10 Painel React
- [x] 11 k6 contra o falso, números no README
- [x] 12 Auditoria ponytail, README, PR

## Depois da entrevista (P1/P2)
- [ ] Rate limit por tokens estimados com reconciliação pelo `usage`
- [ ] Limite de concorrência por cliente
- [ ] Relatório comparativo de shadow mais rico (diferença de conteúdo, não só tamanho)
- [ ] Métricas com o client oficial do Prometheus + OpenTelemetry
- [ ] Postgres + Redis (cache e rate limit compartilhados entre instâncias)
- [ ] Deploy no Cloud Run com Secret Manager
- [ ] Fanout de stream ao vivo para os seguidores do singleflight
- [ ] Fila de jobs adiáveis para rodar fora do pico
- [ ] Cache semântico, com limiar alto e medição de acertos falsos

## Dívida deliberada (`// ponytail:`)
Colhida dos comentários `ponytail:` em 09/10. Cada linha tem o gatilho para pagar.

| Onde | Atalho | Pagar quando |
|---|---|---|
| `internal/httpapi/servidor.go` | Orçamento (gasto do mês) em memória | Mais de uma instância → Redis |
| `internal/httpapi/chat.go` | Seguidores do singleflight recebem replay, não stream ao vivo | TTFT de requisições cacheáveis importar → fanout |
| `internal/httpapi/relay.go` | Um restaurador LGPD para todas as choices | Cliente usar `n > 1` |
| `internal/httpapi/relay.go` | Content multimodal (partes) não é mascarado | Cliente mandar imagem ou partes |
| `internal/httpapi/metricas.go` | Contadores próprios, sem labels | Prometheus/Grafana de verdade → client oficial |
| `internal/gateway/roteador.go` | `Estado()` sem ocupação do semáforo | Junto com as métricas Prometheus |
| `internal/cache/cache.go` | Mutex global | Disputa aparecer no profile → shards |
| `internal/armazenamento/uso.go` | Relatório varre a tabela bruta | Consulta lenta no profile → `uso_minuto` |
| `.claude/docs/arquitetura/frontend.md` | Sem react-router | Rotas aninhadas ou links profundos |
