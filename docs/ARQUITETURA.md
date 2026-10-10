# Comporta — Arquitetura de referência

> Documento de decisões do projeto. Pesquisa feita em 09/10/2026. O CLAUDE.md aponta para cá:
> quando uma decisão mudar, atualize este arquivo junto com o código.

## 1. Resumo das decisões

| Tema | Decisão | Por quê |
|---|---|---|
| Servidor HTTP | `net/http` da biblioteca padrão | streaming SSE, HTTP/2, `context` e `httptest` nativos; o gargalo é o provedor (segundos), não o parser HTTP (microssegundos) |
| Cliente para o provedor | um `http.Client` por provedor com `Transport` ajustado (`MaxIdleConnsPerHost` alto) | o padrão é **2** conexões ociosas por host, o que destrói o reuso de conexões sob carga |
| Formato da API | compatível com OpenAI (`/v1/chat/completions`) | adoção trocando só a base URL |
| Corpo da requisição | struct com os campos que o gateway usa + `json.RawMessage` para o resto | o gateway altera alguns campos sem perder os que não conhece |
| Streaming | relay linha a linha com `bufio.Reader` e flush por evento | latência do primeiro token preservada; restauração de PII e captura de `usage` no caminho |
| Retry e fallback | só **antes do primeiro byte** enviado ao cliente | depois que o stream começou, não dá para "voltar atrás" |
| Proteção do provedor | semáforo de concorrência por modelo + limite por cliente + circuit breaker (`sony/gobreaker/v2`) | a DeepSeek limita concorrência por conta (seção 3) |
| Rate limit | MVP: **requisições/min** por cliente (token bucket, `x/time/rate`). Próximo passo: tokens estimados com reconciliação pelo `usage` | requisições de LLM variam de 10 a 100 mil tokens; o MVP limita volume, o orçamento (402) limita gasto |
| Cache do gateway | exato, **opt-in**, LRU + TTL em memória + `singleflight` | modelos com raciocínio ativo por padrão não são determinísticos |
| Cache do provedor | preservar prefixos estáveis e **medir** `prompt_cache_hit_tokens` | o cache de prefixo da DeepSeek é automático e barateia muito a entrada |
| Registro de uso | channel com buffer + gravação em lote (SQLite WAL); descarta e conta se lotar | nada de escrita em banco no caminho crítico |
| Configuração quente | snapshot imutável em `atomic.Pointer`, trocado quando o admin altera | leitura sem lock em toda requisição |
| Front | React + Vite + TS, TanStack Query, Recharts; servido pelo próprio Go com `//go:embed` | um único binário para deploy |
| Playground com streaming | `fetch` + `ReadableStream` (não `EventSource`) | `EventSource` só faz GET e não envia header de autorização |
| Deploy | container distroless; Cloud Run (GCP) | mesmo ambiente da vaga |

### Estado em 09/10/2026 (branch `feat/mvp`)
As fases 1 e 2 e o modo shadow da fase 3 estão implementados. Diferenças em relação ao plano acima:
- **Provedores:** um único cliente HTTP compatível com OpenAI (`internal/provedor`), sem interface `Provedor`. O falso é um `http.Handler` do mesmo protocolo.
- **Semáforo:** por alias (`concorrencia` no `comporta.json`), liberado só quando o corpo da resposta é fechado (fim do stream).
- **Breaker:** abre com ≥ 5 chamadas e ≥ 50% de falhas; o retry fica dentro dele; 4xx do cliente conta como sucesso; cancelamento é excluído.
- **Singleflight:** o líder lê a resposta inteira e todos recebem replay (sem fanout ao vivo).
- **Shadow:** só no caminho sem cache; chamada sem stream, contexto próprio, comparação em `/admin/uso`. No desligamento (prazo de 8 s, abaixo dos 10 s do Cloud Run) os shadows presos são cancelados.
- **Sem `Repositorio`, sem `uso_minuto`:** o relatório varre a tabela bruta (dívida registrada em `.claude/docs/backlog.md`).
- **Números medidos:** README e `.claude/docs/features/11-carga.md`.

## 2. O que dá para aprender com os gateways existentes

- **LiteLLM** (Python) é o mais usado; **Bifrost** (Go, Apache 2.0) e **Helicone** (Rust) nasceram focados em desempenho; **Portkey** (TypeScript) e **Kong/Envoy AI Gateway** vêm do mundo de API gateways.
- O Bifrost separa **núcleo** (`core/`: provedores e esquemas), **transporte** (`transports/bifrost-http`), **armazenamento** (`framework/`: config, logs, vetores) e **plugins** (governança com chaves virtuais e orçamentos, cache semântico, telemetria). É uma boa referência de organização: a Comporta segue a mesma ideia com pacotes `internal/`.
- O Bifrost publica um **overhead de ~1 ms por requisição** contra ~40 ms do LiteLLM, medido com um provedor falso de 60 ms. São números do próprio fornecedor e o artigo que os repercute tem viés, mas as **lições de engenharia** citadas são consistentes com o que se sabe de Go:
  1. registrar logs de forma **assíncrona e em lote**;
  2. **reduzir alocações** por requisição (pools de buffers);
  3. **ajustar o pool de conexões**: com o transporte padrão, o reuso caiu de ~90% para ~40% sob carga, somando 50–100 ms por requisição.
- **Meta da Comporta:** medir e publicar o próprio overhead (p50/p99) contra o provedor falso. Um número honesto e reproduzível vale mais que um número grande.

## 3. O que a API da DeepSeek impõe ao projeto

Fonte: documentação oficial (api-docs.deepseek.com), consultada em 09/10/2026. **Preços e nomes mudam: tudo isso vai em configuração, nunca no código.**

| Fato | Consequência no gateway |
|---|---|
| Modelos atuais: `deepseek-flash` (V4.1-Flash) e `deepseek-v4-pro`; 1M de contexto | aliases próprios (`rapido`, `forte`) mapeados para modelos na config |
| Modo de raciocínio (`thinking`) **ligado por padrão**; `temperature` não tem efeito nesse modo; resposta traz `reasoning_content` | o cache não pode depender de `temperature: 0`; cache é opt-in. Registrar `reasoning_tokens` no custo |
| **Cache de prefixo automático**: `usage.prompt_cache_hit_tokens` e `prompt_cache_miss_tokens`; acerto exige prefixo idêntico | não reordenar mensagens; mascaramento de PII **determinístico** (mesmo texto → mesmo resultado); mostrar a economia no painel |
| Preços por 1M tokens com **entrada em cache muito mais barata** e **horário de pico** (fora do pico custa metade; pico: 01–04h e 06–10h UTC em dias úteis) | motor de custo sabe a hora; métrica "economia fora do pico"; fila de jobs adiáveis (fase 3) |
| **Limite de concorrência por conta**: 2.500 (`deepseek-flash`) e 500 (`deepseek-v4-pro`); acima disso, 429 | semáforo por modelo um pouco abaixo do limite + limite por cliente, para um cliente não ocupar tudo |
| Campo `user_id` (`[a-zA-Z0-9\-_]+`, sem dado pessoal) para limites por usuário | enviar um hash do ID do cliente, nunca o nome |
| Enquanto espera, o servidor manda **linhas vazias** (sem stream) ou **comentários SSE** `: keep-alive` (com stream); fecha se a inferência não começar em **10 min** | o parser ignora keep-alives; o gateway tem o **seu** timeout até o primeiro token (ex.: 30 s) e não conta keep-alive como progresso |
| `stream_options: {include_usage: true}` faz o último chunk trazer o `usage`; o stream termina com `data: [DONE]` | o gateway injeta `include_usage` para medir custo de todo stream |
| Erros: 400, 401, 402 (saldo), 422, **429**, **500**, **503** | retry só em 429/500/503/rede, respeitando `Retry-After`; 401/402 viram alerta para o admin, não retry |

## 4. Caminho de uma requisição

```
cliente ──▶ [1] recover + request ID
            [2] autenticação: hash(ck_...) → cliente (snapshot em memória)
            [3] limite de corpo + decode (campos conhecidos + RawMessage)
            [4] admissão: rate limit por tokens estimados + orçamento   ──✗ 429 / 402
            [5] mascaramento LGPD (se ligado para o cliente)
            [6] cache exato (opt-in) ── hit ──▶ responde (replay em SSE se stream)
                    │ miss
            [7] singleflight por chave de cache (só requisições cacheáveis)
            [8] roteador: alias → lista ordenada [principal, fallback...]
            [9] por destino: semáforo do modelo → circuit breaker → chamada
                    └ retry com backoff+jitter ANTES do primeiro byte; senão próximo destino
           [10] relay: lê SSE do provedor, ignora keep-alive, restaura PII,
                captura usage do último chunk, escreve e dá flush ao cliente
           [11] evento de uso → channel com buffer → gravador em lote
```

Ordem importa: autenticação e admissão vêm **antes** de qualquer trabalho caro; o mascaramento vem **antes** do cache (o cache nunca vê dado pessoal); o retry fica **dentro** do breaker (para o breaker contar a tentativa final).

## 5. Desempenho no caminho quente

- **Transporte:** clonar `http.DefaultTransport` e ajustar `MaxIdleConns`, `MaxIdleConnsPerHost` e `MaxConnsPerHost` (100–1000, conforme o limite do provedor), `IdleConnTimeout` 90 s, `ForceAttemptHTTP2: true`. **Sem** `Client.Timeout` global, porque ele mata streams longos; os prazos vêm do `context` e dos timeouts por etapa (seção 7).
- **JSON:** decodificar só o necessário. No stream, não fazer `Unmarshal` completo de cada chunk: só os chunks que precisam de restauração de PII e o último (com `usage`).
- **Buffers:** `sync.Pool` para os buffers de leitura/escrita do relay.
- **Configuração:** `atomic.Pointer[Snapshot]`, sem mutex na leitura.
- **Uso assíncrono:** `chan Evento` com buffer (ex.: 10 mil). Se lotar, **descarta e incrementa uma métrica**: perder um registro de uso é aceitável, travar a API não.
- **Runtime:** `GOMEMLIMIT` abaixo do limite do container.
- **Medição:** `BenchmarkOverhead` (gateway + provedor falso de latência zero) com `-benchmem`, e k6 com provedor falso de 60 ms para comparar com o número publicado pelo Bifrost.

## 6. Cache

**Duas camadas, com papéis diferentes:**

1. **Cache exato do gateway** (resposta inteira):
   - Opt-in: header `X-Comporta-Cache: ligado` ou política do cliente. Com raciocínio ativo, a mesma pergunta gera respostas diferentes; reaproveitar precisa ser escolha explícita.
- **`X-Comporta-Enviado`** (resposta, só para cliente com LGPD): a última mensagem do usuário como saiu para o provedor, já mascarada, em base64 e cortada em 2 KB. Vai num cabeçalho para não mudar o corpo no formato OpenAI que os SDKs esperam; o Playground usa para mostrar o antes e depois.
   - Chave: `sha256(escopo + modelo + mensagens já mascaradas + parâmetros relevantes)`. O escopo padrão é **por cliente** (evita um cliente ver resposta de outro).
   - LRU com TTL e limite de memória (o LRU genérico do laboratório da apostila serve de base).
   - Respostas em stream são guardadas inteiras e devolvidas em **replay SSE**.
   - `singleflight` na mesma chave: N pedidos simultâneos iguais viram 1 chamada paga. Atenção: com stream, o primeiro a chegar recebe ao vivo; os demais esperam a resposta completa ou assinam o mesmo stream (versão avançada).
2. **Cache de prefixo da DeepSeek** (automático, só da entrada):
   - O gateway **não atrapalha**: não reordena mensagens e mascara PII de forma determinística.
   - O painel mostra `prompt_cache_hit_tokens` e quanto isso economizou.

**Cache semântico** (perguntas parecidas): fica para depois. Exige embeddings (a documentação da DeepSeek que consultei só cobre chat, então seria preciso outro provedor ou um modelo local) e tem risco real de devolver resposta errada. Se for feito, com limiar alto e medição de acertos falsos.

## 7. Resiliência

| Mecanismo | Configuração inicial |
|---|---|
| Timeout de conexão | 5 s |
| Timeout até o primeiro token | 30 s (keep-alive não conta) |
| Timeout entre chunks | 60 s sem dados úteis |
| Prazo total | herdado do `context` do cliente (desconectou → cancela o upstream) |
| Retry | até 2, só em 429/500/503/erro de rede, **só antes do primeiro byte**, backoff exponencial com jitter, `Retry-After` respeitado, dentro do prazo |
| Circuit breaker | `gobreaker/v2` por destino (provedor+modelo); abre com taxa de falha alta numa janela; `IsSuccessful` trata 4xx do cliente (400/422) como sucesso, porque não indicam provedor doente |
| Fallback | lista ordenada por alias; breaker aberto → próximo da lista |
| Concorrência | semáforo por modelo abaixo do limite da conta + máximo simultâneo por cliente; fila curta com prazo e 429 + `Retry-After` quando passar |

Mapeamento de erros para o cliente: 401 (chave inválida), 402 (orçamento esgotado), 429 (limite do cliente ou do provedor), 502 (resposta inválida do provedor), 503 (breaker aberto / sem destino disponível), 504 (timeout). Sempre `{"erro": {"codigo", "mensagem"}}`, sem vazar detalhes internos.

## 8. Rate limit, orçamento e custo

- **Pré-voo:** estimar tokens de entrada (heurística `caracteres/3`, documentada, ou um tokenizer depois) + reservar `max_tokens` de saída. Recusar cedo se estourar o limite por minuto ou o orçamento.
- **Reconciliação:** ao fim, ajustar com o `usage` real. Medir a diferença entre estimado e real e alertar se passar de ~15%.
- **Custo:** `(miss × preço_miss + hit × preço_hit + saída × preço_saída)` com a tabela da hora (pico/fora do pico), tudo vindo da configuração. Guardar em micro-dólares (`int64`), nunca `float` para dinheiro.
- **Métricas de economia** no painel: cache do gateway, cache de prefixo da DeepSeek e horário fora do pico.

## 9. LGPD

Ver regras no CLAUDE.md. Pontos de arquitetura:

- Mascaramento **determinístico por requisição**: marcadores numerados pela ordem de aparição. Mesmo texto → mesmo texto mascarado, o que preserva os dois caches.
- Validação de dígito de CPF/CNPJ e Luhn para cartão (menos falsos positivos).
- Restauração no stream com buffer pequeno para marcadores que chegam partidos entre chunks.
- O mapa marcador → valor real só existe na memória da requisição.

## 10. Dados

SQLite (`modernc.org/sqlite`, sem cgo) em modo **WAL**, com interface `Repositorio` para trocar por Postgres depois.

| Tabela | Conteúdo |
|---|---|
| `clientes` | id, nome, limites, orçamento mensal, política de cache e de LGPD, modelos permitidos |
| `chaves` | id, cliente_id, prefixo visível (`ck_ab12…`), hash SHA-256, criada_em, revogada_em |
| `uso` | cliente, modelo, destino, tokens (hit, miss, saída, raciocínio), custo em micro-dólares, latência total, tempo até o 1º token, cache (gateway/prefixo), status, timestamp. **Sem conteúdo das mensagens** |
| `uso_minuto` | agregados por cliente/modelo/minuto para o painel não varrer a tabela bruta |

Escrita em lote (ex.: a cada 500 eventos ou 1 s). Agregação feita no mesmo lote.

## 11. Observabilidade

- **Prometheus** (`/metrics`): requisições, erros e latência por cliente/modelo/destino (labels de baixa cardinalidade: IDs de cliente sim, nunca IDs de requisição); tempo até o primeiro token; estado do breaker; ocupação dos semáforos; hits de cache; eventos de uso descartados.
- **slog** em JSON com `request_id`.
- **pprof** em porta interna.
- OpenTelemetry na fase 3.

## 12. Front-end

- **Vite + React + TypeScript**, React Router, **TanStack Query** (cache de dados do servidor e atualização a cada poucos segundos), **Recharts** para os gráficos, Tailwind + shadcn/ui para não perder tempo com componentes base.
- Telas: visão geral (gasto, economia, p95, taxa de erro), clientes e chaves (criar, revogar, mostrar a chave uma única vez), uso detalhado com filtros, estado dos provedores (breaker), playground.
- **Playground:** `fetch` com `ReadableStream` para ler o SSE de um POST autenticado; mostrar separadamente o raciocínio (`reasoning_content`) e a resposta, o tempo até o primeiro token e o custo da chamada.
- **Autenticação do admin:** login com o `ADMIN_TOKEN` → cookie de sessão `HttpOnly; Secure; SameSite=Strict`. O token nunca fica no `localStorage`. Login limitado a 5 tentativas por IP (consumidas na entrada); o IP vem do último `X-Forwarded-For` só com `K_SERVICE` ou `TRUST_PROXY=1`, senão do endereço da conexão.
- **Build:** `web/dist` embutido no binário Go com `//go:embed`; o Go serve os arquivos e faz fallback para `index.html` nas rotas do React.

## 13. Testes

| Tipo | O quê |
|---|---|
| Unitário (table-driven) | estimativa de tokens, cálculo de custo com pico/fora do pico, chave de cache, mascaramento e restauração, mapeamento de erros |
| Concorrência (`-race`) | singleflight: N pedidos iguais → 1 chamada; semáforo nunca passa do limite; cache e snapshot sob escrita concorrente |
| Provedor real contra `httptest.NewServer` | imita a DeepSeek: stream com `: keep-alive`, `usage` no último chunk, `[DONE]`, 429 com `Retry-After`, 503 |
| Fuzz | parser SSE; mascarar→restaurar devolve o original |
| Benchmark | overhead do gateway com `-benchmem` |
| Carga (k6) | carga constante, pico, e **caos**: provedor falso com 50% de 503 para mostrar breaker e fallback funcionando |

O **provedor falso** é peça central: latência, tempo até o primeiro token, velocidade de tokens e taxa de erro configuráveis.

## 14. Deploy (GCP)

- Imagem distroless nonroot com o binário único (API + painel).
- **Cloud Run**: ajustar a concorrência por instância, o timeout da requisição para comportar streams longos e um mínimo de instâncias para evitar partida a frio; segredos no **Secret Manager**.
- Com mais de uma instância, o rate limit e o cache em memória deixam de ser globais: é o gatilho para Redis (Memorystore) na fase 3.

## 15. Fases

**Fase 1 — até 13/10 (entrevista):** provedor falso + DeepSeek; `/v1/chat/completions` com e sem stream; chaves por cliente; rate limit; semáforo por modelo; retry + breaker; cache exato opt-in + singleflight; uso em lote; painel com visão geral, clientes e playground; k6 com números no README.

**Fase 2:** mascaramento LGPD (sem stream e depois com stream); orçamento (402); fallback entre modelos; estimativa de tokens com reconciliação; métricas Prometheus.

**Fase 3:** modo shadow com relatório comparativo; fila de jobs adiáveis para rodar fora do pico; Redis para estado compartilhado; Postgres; OpenTelemetry; Cloud Run.

## 16. Fontes

- DeepSeek, documentação oficial: Chat Completion, Context Caching, Models & Pricing, Rate Limit e Error Codes (api-docs.deepseek.com).
- Bifrost: repositório github.com/maximhq/bifrost e benchmarks publicados em getmaxim.ai/bifrost (números do fornecedor).
- dev.to, "We benchmarked 5 LLM gateways at 5000 RPS" (lições sobre logs assíncronos, alocações e pool de conexões; texto com viés pró-Bifrost).
- MLflow, "Gateway LLM rate limits" (limites por tokens, reconciliação, tratamento de 429).
- tianpan.co, "Multi-tenant LLM noisy neighbor isolation" (admissão, prioridades, load shedding).
- LoginRadius, "Tune the Go HTTP client for high performance" (padrões do `http.Transport`).
- pkg.go.dev/github.com/sony/gobreaker/v2.
