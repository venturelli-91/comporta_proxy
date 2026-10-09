# Backlog

Prioridades da softhouse: P0 (prazo da entrevista), P1 (semana), P2 (melhoria).

## MVP — até 13/10 (P0)
- [x] 01 Config em JSON e custo com horário de pico
- [x] 02 Provedor: cliente compatível com OpenAI + provedor falso
- [x] 03 Retry com backoff, jitter e Retry-After
- [x] 04 Roteador: semáforo, circuit breaker, fallback
- [ ] 05 Cache LRU com TTL
- [ ] 06 LGPD: mascaramento + restauração (inclusive em stream)
- [ ] 07 Armazenamento: clientes, chaves por hash, uso em lote
- [ ] 08 API HTTP: chat (com e sem stream), auth, rate limit, orçamento, cache + singleflight, shadow, admin, métricas
- [ ] 09 Binário + Docker + compose
- [ ] 10 Painel React
- [ ] 11 k6 contra o falso, números no README
- [ ] 12 Auditoria ponytail, README, PR

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
Preenchida pelo `ponytail:ponytail-debt` no fim do MVP.
