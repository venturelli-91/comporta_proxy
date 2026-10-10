# Arquitetura do backend

Fonte da verdade das decisões: `docs/ARQUITETURA.md`. Contratos: spec e plano em `docs/superpowers/`.
Esta página é o mapa para navegar o código.

## Pacotes
| Pacote | Responsabilidade | Depende de |
|---|---|---|
| `cmd/comporta` | Lê o ambiente, liga as dependências, sobe o falso se não houver `DEEPSEEK_API_KEY`, faz o shutdown gracioso | todos |
| `internal/gateway` | Config (`comporta.json`), custo, roteador (semáforo → breaker → retry → fallback) | `provedor`, `resiliencia`, gobreaker |
| `internal/provedor` | `Cliente` HTTP compatível com OpenAI (DeepSeek ou outro) + `Falso` (`http.Handler` do mesmo protocolo) | stdlib |
| `internal/resiliencia` | `Retry` com backoff exponencial, jitter e `Retry-After` | stdlib |
| `internal/cache` | LRU com TTL e limite em bytes | stdlib |
| `internal/lgpd` | Mascarar e restaurar CPF, CNPJ, e-mail, telefone e cartão; restaurador de stream | stdlib |
| `internal/armazenamento` | SQLite: clientes, chaves (hash), uso em lote, relatórios | modernc sqlite |
| `internal/httpapi` | Rotas, auth, admissão, chat (com e sem stream), admin, métricas, painel embutido | todos acima, x/time/rate, x/sync |

O grafo é acíclico: `httpapi` → `gateway` → `provedor`/`resiliencia`. O `resiliencia` não importa o
`provedor`. Ele só enxerga as interfaces `Repetivel()` e `Espera()`, que o `ErroHTTP` implementa.

## Caminho de `POST /v1/chat/completions`
```
recover + X-Request-Id
 → auth: Bearer ck_... → sha256 → cliente (snapshot atomic.Pointer)
 → admissão: token bucket (429 + Retry-After) · orçamento do mês (402) · modelo liberado (403)
 → corpo ≤ 1 MiB → map[string]json.RawMessage
 → LGPD: mascara messages[].content (se o cliente tiver ligado)
 → cache opt-in: chave sha256(cliente + corpo mascarado); hit → responde
 → singleflight na mesma chave (só cacheáveis)
 → roteador: destinos [principal, fallback...]; cada um: semáforo → breaker → retry
 → relay: JSON inteiro ou SSE linha a linha (ignora keep-alive, restaura PII, captura usage)
 → evento de uso → canal → gravador em lote (SQLite)
 → shadow (se configurado): goroutine com contexto próprio, registra a comparação
```

## Concorrência
| Estado | Proteção |
|---|---|
| Snapshot de clientes e chaves | `atomic.Pointer`, trocado inteiro a cada mutação do admin |
| Limitadores por cliente | `sync.Mutex` + mapa (criação preguiçosa) |
| Cache | `sync.Mutex` (o LRU mexe na lista até no Get) |
| Semáforo por modelo | canal com buffer |
| Contadores de métricas | `atomic.Int64` |
| Uso | `chan Evento` com buffer; descarta quando enche |
