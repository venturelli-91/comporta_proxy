# Comporta — Design do MVP (Fase 1 + diferenciais do pitch)

Data: 09/10/2026. Entrevista: 14/10/2026. Base: `CLAUDE.md` e `docs/ARQUITETURA.md`.

## 1. Entendimento

**O que foi pedido:** um gateway de LLM em Go, público e medido, que sustente o pitch de 30 segundos do
`CLAUDE.md` numa entrevista para uma vaga de proxy em Go na GCP. Documentação completa (spec, plano,
backlog, checklist, changelog, arquitetura). Simplicidade verificada com o ponytail o tempo todo.

**Critério de sucesso:** cada frase do pitch tem código, teste e número por trás:

| Frase do pitch | Prova |
|---|---|
| mascara CPF, e-mail, telefone e restaura na volta | `internal/lgpd` com table-driven + fuzz; teste de ponta a ponta com o provedor falso ecoando o texto mascarado |
| cada aplicação tem sua chave, limite e orçamento | chaves `ck_` com hash SHA-256; 401, 429 com `Retry-After`, 402 |
| requisições iguais cacheadas e deduplicadas | teste concorrente: N pedidos iguais → 1 chamada ao provedor |
| timeout, retry e circuit breaker | testes com o falso devolvendo 503; fallback para o próximo destino |
| modo shadow | segundo modelo chamado em segundo plano; comparação no painel |
| medi com k6 | `carga/carga.js` contra o falso; números no README |

**Suposições (minhas, não do usuário):**
- Preços da DeepSeek no `comporta.json` são exemplos; a tabela oficial deve ser conferida antes de uso real.
- Uma instância só (cache, rate limit e orçamento em memória). Redis fica para a fase 3.
- Admin único (`ADMIN_TOKEN`), sem multiusuário.

## 2. Abordagens consideradas

1. **Um cliente HTTP compatível com OpenAI + provedor falso que fala o mesmo protocolo (escolhida).**
   A DeepSeek é compatível com OpenAI, então "deepseek" e "openaicompat" são o mesmo código com outra
   `base_url`. O falso é um `http.Handler` que imita a DeepSeek (keep-alive, `usage` no último chunk,
   `[DONE]`, 503 aleatório). Testes e k6 passam pelo **mesmo** caminho de rede do provedor real.
2. Interface `Provedor` com três implementações (o que o spec inicial sugeria). Três códigos para
   manter, e o falso em memória pularia justamente o código de rede que mais quebra.
3. Proxy reverso genérico (`httputil.ReverseProxy`). Bom para repasse cego, ruim para mascarar,
   cachear e medir tokens no meio do stream.

## 3. Arquitetura

```
cmd/comporta/main.go       config por ambiente, liga tudo, sobe o falso se não houver DEEPSEEK_API_KEY, shutdown gracioso
internal/gateway/          config (comporta.json), custo (micro-dólares, pico), roteador (semáforo → breaker → retry → fallback)
internal/provedor/         Cliente HTTP compatível com OpenAI (transporte ajustado) + Falso (http.Handler)
internal/resiliencia/      Retry com backoff exponencial + jitter + Retry-After
internal/cache/            LRU com TTL e limite em bytes (container/list)
internal/lgpd/             Mascarador e restaurador (inclusive em stream)
internal/armazenamento/    SQLite (modernc): clientes, chaves (hash), uso; gravador em lote
internal/httpapi/          rotas, middlewares, chat (com/sem stream), admin, métricas, painel embutido
web/                       painel React (Vite + TS + TanStack Query + Recharts + Tailwind)
carga/                     k6
```

### Caminho de `POST /v1/chat/completions`

1. `recover` + `X-Request-Id` + log estruturado (sem conteúdo).
2. Autenticação: `Bearer ck_...` → SHA-256 → cliente no snapshot (`atomic.Pointer`, sem lock).
3. Admissão: token bucket por cliente (`x/time/rate`, req/min) → 429 + `Retry-After`;
   orçamento mensal em micro-dólares → 402; modelo fora da lista do cliente → 403.
4. Corpo limitado (1 MiB), decodificado em `map[string]json.RawMessage` (campos desconhecidos preservados).
   `model` é um alias da config (`rapido`, `forte`).
5. LGPD (se ligado no cliente): mascara `messages[].content` (texto). O mapa vive só na requisição.
6. Cache (opt-in: header `X-Comporta-Cache: ligado` ou política do cliente): chave
   `sha256(cliente + corpo mascarado)`. Hit → responde (JSON ou replay SSE). Miss → `singleflight`
   na mesma chave; o líder busca a resposta inteira e grava; os demais recebem os mesmos bytes.
7. Roteador: alias → destinos `[principal, fallback...]`. Por destino: semáforo do modelo (espera
   curta, senão 429) → circuit breaker (`gobreaker/v2`) → retry (até 2, só 429/5xx/rede, antes do
   primeiro byte) → próximo destino se o provedor falhou.
8. Relay: sem stream, lê o JSON inteiro, restaura PII e devolve. Com stream, lê o SSE linha a linha,
   ignora `: keep-alive`, restaura PII com buffer para marcador partido, captura `usage` do último
   chunk, escreve e dá flush. Watchdog: 30 s até o primeiro dado útil, 60 s entre dados.
9. Uso: evento → `chan` com buffer (10 mil) → gravador em lote (500 eventos ou 1 s). Canal cheio:
   descarta e conta na métrica. Shadow (se configurado no alias): goroutine com contexto próprio
   chama o modelo sombra sem stream e registra um evento `shadow=1` para comparação.

### Erros
`{"erro": {"codigo", "mensagem"}}`. 400 corpo inválido; 401 chave; 402 orçamento; 403 modelo não
liberado; 413 corpo grande; 429 limite (cliente ou provedor); 502 resposta inválida/erro do provedor;
503 breaker aberto ou sem destino; 504 timeout; 500 interno sem detalhes.

### Dados
`clientes(id, nome, rpm, orcamento_micro, cache, lgpd, modelos, criado_em)`,
`chaves(id, cliente_id, prefixo, hash UNIQUE, criada_em, revogada_em)`,
`uso(ts, cliente_id, request_id, modelo, destino, status, tokens..., custo_micro, economia_micro,
latencia_ms, ttft_ms, cache, stream, shadow, pii, tamanho_resposta)`. Sem conteúdo de mensagens.
SQLite em WAL. Agregados (`uso_minuto`) ficam para quando a varredura pesar.

### Admin e painel
`/admin/login` (cookie `HttpOnly; SameSite=Strict`, ou `Authorization: Bearer ADMIN_TOKEN`),
`/admin/clientes` CRUD, `/admin/clientes/{id}/chaves` gerar/listar/revogar (chave inteira só na
criação), `/admin/uso?de=&ate=&cliente=` (resumo, por cliente, por modelo, série por minuto, p95),
`/admin/provedores` (estado dos breakers). Painel: Visão geral, Clientes e chaves, Shadow,
Playground (fetch + ReadableStream com uma chave `ck_` colada pelo usuário).
`/healthz`, `/readyz`, `/metrics` (formato Prometheus, contadores próprios).

## 4. Testes
- Table-driven: custo (pico/fora do pico/cache de prefixo), LGPD (casos reais e falsos positivos), retry.
- Fuzz: mascarar → restaurar devolve o original; restaurador em stream com cortes arbitrários.
- Concorrência com `-race`: singleflight N→1, cache, semáforo nunca passa do limite.
- Integração com `httptest` + falso: 401, 429, 402, sem stream, stream, cache, LGPD ida e volta,
  fallback com 503, breaker.
- Benchmark do overhead do gateway (`-benchmem`). k6: constante, cache, caos (50% de 503).

## 5. Fora do escopo agora (no backlog)
Prometheus client e OpenTelemetry, Postgres e Redis, deploy no Cloud Run, rate limit por tokens
estimados com reconciliação, limite de concorrência por cliente, cache semântico, fanout de stream
ao vivo para seguidores do singleflight, fila de jobs fora do pico.
