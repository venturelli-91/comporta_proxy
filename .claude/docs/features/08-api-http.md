# 08 — API HTTP (chat, admin, métricas)

**Pacote:** `internal/httpapi` · **Painel embutido:** `web/embed.go`

## O que faz
| Rota | Função |
|---|---|
| `POST /v1/chat/completions` | Compatível com OpenAI, com e sem `stream` |
| `GET /v1/models` | Aliases liberados para a chave |
| `GET /healthz`, `/readyz`, `/metrics` | Sondas e contadores no formato Prometheus |
| `/admin/*` | Login (cookie), clientes, chaves, uso, provedores, modelos |
| `/` | Painel React embutido (fallback para `index.html`) |

Caminho do chat: request ID → `Bearer ck_` → hash → snapshot → rate limit (429 + `Retry-After`) → orçamento (402) →
corpo ≤ 1 MiB (413) → alias (400/403) → LGPD → `include_usage` no stream → cache opt-in + singleflight
**ou** shadow + chamada direta → relay → custo → evento de uso.

## Por que assim (decisões)
- **Snapshot em `atomic.Pointer`:** autenticar não trava nada, e o admin troca o snapshot inteiro a cada mudança. Uma chave revogada cai na hora (teste `TestRevogacaoValeNaHora`).
- **Corpo como `map[string]json.RawMessage`:** campos que o gateway não conhece (`tools`, `response_format`...) seguem intactos para o provedor.
- **Cabeçalhos do stream só no primeiro dado útil:** até lá ainda dá para devolver 504 de verdade. Keep-alive não reinicia o watchdog.
- **Cliente desconectou = 499:** o contexto da requisição vai até o provedor e a chamada paga é cancelada.
- **Singleflight com `context.WithoutCancel`:** se o "líder" desconectar, os outros que esperavam a mesma resposta não perdem a chamada.
- **Quem paga no singleflight:** o líder registra o custo; os seguidores registram economia, como um cache hit.
- **Shadow só sem cache**, em goroutine com contexto próprio, rastreada por `WaitGroup` para o shutdown esperar.
- **Cookie do admin guarda o SHA-256 do token**, não o token. `HttpOnly; SameSite=Strict`, comparado com `subtle.ConstantTimeCompare`.
- **Chave `ck_` buscada pelo hash num mapa:** o tempo da busca não vaza a chave, porque quem ataca não controla o hash.
- Ponytail: seguidores do singleflight recebem replay em vez de stream ao vivo; métricas sem o client do Prometheus; content multimodal não é mascarado; um restaurador de stream para todas as choices. Tudo marcado no código.

## Números (09/10, notebook do Aurélio, provedor falso no loopback)
`go test -run x -bench Overhead -benchmem ./internal/httpapi/`
| Caso | ns/op | B/op | allocs/op |
|---|---|---|---|
| direto | ~187 000 | 27 238 | 260 |
| com LGPD | ~215 000 | 38 231 | 305 |

Isso inclui o salto HTTP real até o falso. O mascaramento custa por volta de 28 µs por requisição.

## Como testar
`go test ./internal/httpapi/ -v`: 401, 429 + Retry-After, 402, 403, 400, 413, sem stream, stream, cache hit, cache opt-in,
singleflight (10 → 1), LGPD ida e volta (com e sem stream), fallback, revogação, shadow, desconexão (499), watchdog (504),
`/v1/models`, `/metrics` e admin sem token. Testes de mutação confirmaram que os testes de LGPD e de singleflight
detectam a regressão.

## Go vs TypeScript
- `http.ServeMux` com padrões `"POST /v1/chat/completions"` e `{id}` (Go 1.22+) substitui o roteador do Express.
- Middleware é só uma função que recebe e devolve `http.HandlerFunc`, sem `next()`.
- `context.WithValue` é como o `res.locals` do Express, mas imutável e encadeado.
- `http.NewResponseController(w).Flush()` corresponde ao `res.flush()` do SSE no Node.

## Exercício
Escreva o teste de que um cliente com orçamento estourado **não** consegue passar pelo cache (o 402 vem antes). Depois mova a checagem de orçamento para depois do cache e veja o teste quebrar.

## Pergunta de entrevista
"Dez requisições iguais chegam ao mesmo tempo e o cliente da primeira desconecta. O que acontece com as outras nove?"

## Correções da revisão final (09/10)
O review independente do branch achou 7 pontos importantes. Todos foram corrigidos com um teste que falhou antes:
| Achado | Correção | Teste |
|---|---|---|
| Stream travava sem prazo até o provedor mandar cabeçalhos | `vigia`: o watchdog começa antes do `Chamar` e cancela com `errTimeout` | `TestWatchdogAntesDosCabecalhos` |
| Caminho de cache/singleflight sem watchdog | o líder lê através da mesma `vigia` | `TestWatchdogNoCaminhoDoCache` |
| Breaker ignorava timeouts | só `context.Canceled` (cliente) é excluído; `Enviar` devolve `context.Cause` | `TestBreakerContaTimeout` |
| Marcador literal numa mensagem colidia com o dado real de outra | `lgpd.MascararVarios` reserva o texto de todas as mensagens | `TestMascararVariosNaoColideEntreMensagens` + `FuzzMascararVarios` |
| Chunk de usage com `choices: []` quebrava `chunk.choices[0]` dos SDKs | só repassa se o cliente pediu `include_usage`; o falso agora imita a DeepSeek | `TestUsageSoQuandoOClientePede` |
| Stream cortado registrava custo 0 (furava o orçamento) | estimativa: bytes das mensagens / 4 + 1 token por chunk | `TestStreamCortadoAindaCobra` |
| Shadow sem limite disputava a concorrência da conta | semáforo de 16 shadows; sem vaga, descarta e conta (`comporta_shadow_descartados_total`) | `TestShadowLimitado` |

Bônus da correção: o relay ficou mais simples. O `vigia` cancela o contexto e a leitura passou a ser síncrona, sem a goroutine leitora.
