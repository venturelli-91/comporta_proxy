# Backlog

Prioridades da softhouse: P0 (prazo da entrevista), P1 (semana), P2 (melhoria).

## Segurança do login (P0) — resolvido em 10/10, branch `fix/login-seguranca`
- [x] Corrida (TOCTOU): `Allow()` na entrada; teste com 20 tentativas paralelas
- [x] `X-Forwarded-For` só com proxy confiável (`K_SERVICE` ou `TRUST_PROXY=1`)
- [x] Poda do mapa de tentativas tira só limitadores cheios
- [x] M-5 revogar chave exige o cliente dono
- [x] Cortes de stdlib da auditoria ponytail (`cmp.Or`, `slices.SortFunc`, sem struct `limitador`)

Próximo: deploy no Cloud Run com o provedor falso (antes da entrevista).

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
| `internal/httpapi/admin.go` | Poda do mapa de tentativas é O(n) e só tira os cheios | Ataque com >10 mil IPs reais gastos ao mesmo tempo → LRU com teto |
| `internal/httpapi/auth.go` | Limite de 32 requisições simultâneas igual para todos | Algum cliente precisar de mais → coluna no cliente |
| `internal/httpapi/auth.go` | Sessões do admin em memória | Mais de uma instância → Redis |
| `internal/httpapi/chat.go` | Uso estimado (stream cortado) sem marca no evento | Painel precisar distinguir → coluna nova |
| `internal/httpapi/chat.go` | Seguidores do singleflight recebem replay, não stream ao vivo | TTFT de requisições cacheáveis importar → fanout |
| `internal/httpapi/relay.go` | Um restaurador LGPD para todas as choices | Cliente usar `n > 1` |
| `internal/httpapi/relay.go` | Content multimodal (partes) não é mascarado | Cliente mandar imagem ou partes |
| `internal/httpapi/metricas.go` | Contadores próprios, sem labels | Prometheus/Grafana de verdade → client oficial |
| `internal/gateway/roteador.go` | `Estado()` sem ocupação do semáforo | Junto com as métricas Prometheus |
| `internal/cache/cache.go` | Mutex global | Disputa aparecer no profile → shards |
| `internal/armazenamento/uso.go` | Relatório varre a tabela bruta | Consulta lenta no profile → `uso_minuto` |
| `.claude/docs/arquitetura/frontend.md` | Sem react-router | Rotas aninhadas ou links profundos |

## Achados menores da revisão final (09/10) — adiados
- [x] M-1 `Recarregar` concorrente pode gravar snapshot antigo (revogação desfeita até a próxima mutação): mutex em volta de `Recarregar` (resolvido 10/10)
- [ ] M-2 Lote de uso que falha ao gravar some sem log: logar e somar em `descartados`
- [x] M-3 Shutdown espera shadows sem limite: contexto do servidor cancelado no SIGTERM + prazo de ~8 s (resolvido 10/10)
- [x] M-4 Stream restaura só `delta.content`; `reasoning_content` da DeepSeek fica mascarado (resolvido 10/10)
- [x] M-5 (resolvido 10/10) Revogar chave ignora o cliente do path: `AND cliente_id = ?`
- [x] M-6 Resposta sem stream acima de 16 MiB é truncada e volta 200: ler `max+1` e devolver 502 (resolvido 10/10)
- [ ] M-7 Painel shadow compara tamanho SSE × JSON: medir tamanho do conteúdo
- [x] M-8 (resolvido 09/10: sessão aleatória revogável, limite de login, Secure atrás de proxy, compose sem token padrão) Cookie admin não revogável no servidor, `Secure` atrás do TLS do Cloud Run, sem limite de tentativas, token padrão no compose
- [ ] M-9 Teste de desconexão não confere o status 499 explicitamente
- [ ] M-10 Sessão expirada não volta para o login no painel (invalidar `sessao` num 401)
