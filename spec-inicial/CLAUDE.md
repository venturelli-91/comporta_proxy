# CLAUDE.md — Comporta: gateway de LLM em Go

## Contexto

Projeto de portfólio do Aurélio para uma entrevista na Globo em 14/10/2026.
A vaga é para um **proxy em Go, na GCP**, que está substituindo uma aplicação legada: ele recebe
requisições de várias aplicações web de clientes e direciona buscas de dados nas clouds de forma
mais eficiente.

Eu venho de **TypeScript, React e Python** e estou aprendendo Go agora. O projeto existe para:
1. **Eu aprender Go de verdade** e conseguir explicar cada decisão na entrevista.
2. Ter um produto público, funcionando e com números medidos, que resolva um problema comercial real.

## O produto

**Comporta** é um *gateway de LLM*: um proxy que fica entre as aplicações de uma empresa e os
provedores de IA (DeepSeek primeiro, depois qualquer API compatível com OpenAI).

**Problema comercial:** empresas que colocam IA nos produtos perdem o controle rápido. A chave da API
fica espalhada em vários apps, ninguém sabe quanto cada time gasta, a mesma pergunta é paga
várias vezes, um app com bug consome o orçamento do mês e, quando o provedor cai, tudo cai junto.
Ferramentas como LiteLLM, Portkey, Helicone e Cloudflare AI Gateway existem justamente por isso.

**O que a Comporta faz:**
- Cada aplicação cliente recebe a **sua própria chave** (`ck_...`); a chave real do provedor fica só no servidor.
- **Rate limit e orçamento por cliente** (requisições/minuto e limite de gasto em US$ por mês).
- **Cache** de respostas idênticas + **singleflight** (pedidos iguais simultâneos viram uma única chamada).
- **Timeout, retry com backoff e circuit breaker** por provedor; **fallback** para outro modelo/provedor.
- **Streaming** (SSE) repassado ao cliente token a token.
- **Registro de uso**: tokens, custo estimado, latência, cache hit, por cliente e por modelo.
- **Modo shadow**: manda a mesma requisição para um segundo modelo, devolve só a do principal e
  registra a comparação (custo, latência, tamanho da resposta). Serve para migrar de modelo/provedor
  sem risco, que é o mesmo problema de "modernizar um legado".
- **Proteção de dados pessoais (LGPD)**: antes de enviar ao provedor, detecta e mascara CPF, CNPJ,
  e-mail, telefone com DDD e cartão (`"meu CPF é 123.456.789-09"` → `"meu CPF é [CPF_1]"`), e restaura
  os valores originais na resposta. Ligado/desligado por cliente. Importa ainda mais com provedores
  hospedados fora do Brasil, como a DeepSeek.
- **Painel React**: clientes, chaves, uso, custo, cache hit rate, latência p95, quantos dados pessoais
  foram mascarados, e um playground.

**Paralelo com a vaga:** proxy em Go recebendo requisições de várias aplicações clientes,
roteando para backends externos de forma eficiente (cache, deduplicação, conexões reaproveitadas,
timeouts), com migração segura via shadow. É o mesmo tipo de problema, num domínio fácil de demonstrar.

> **Arquitetura detalhada e decisões técnicas: `docs/ARQUITETURA.md`. Leia antes de implementar
> qualquer parte do caminho da requisição, e atualize o documento quando uma decisão mudar.**

## Stack

- **Back-end:** Go 1.24+, preferindo a biblioteca padrão (`net/http` com roteamento do 1.22,
  `log/slog`, `encoding/json`). Dependências permitidas: `golang.org/x/time/rate`,
  `golang.org/x/sync` (singleflight, errgroup), `github.com/sony/gobreaker/v2`, driver de banco.
  Qualquer outra: me pergunte antes.
- **Banco:** SQLite sem cgo (`modernc.org/sqlite`) no início; interface `Repositorio` para trocar
  por Postgres depois.
- **Front-end:** React + TypeScript + Vite em `web/`, TanStack Query, Recharts, Tailwind + shadcn/ui.
  O build é embutido no binário Go com `//go:embed` (um único artefato de deploy).
- **LLM:** DeepSeek pela API compatível com OpenAI (em 09/10/2026: `deepseek-flash` e
  `deepseek-v4-pro`, raciocínio ligado por padrão, preço menor fora do horário de pico).
  Modelos, preços e janelas de pico vêm de configuração; nunca fixos no código.
- **Testes de carga:** k6 em `carga/`, **sempre contra o provedor falso** (para não gastar dinheiro).
- **Infra:** Docker multi-stage + docker compose. Deploy futuro: Cloud Run (GCP), por ser o ambiente da vaga.

## Arquitetura

```
apps clientes ──(chave ck_...)──▶ Comporta (Go)
                                   ├─ middleware: auth → rate limit → orçamento → log/métricas
                                   ├─ mascarador LGPD (ida) / restaurador (volta)
                                   ├─ cache (memória, TTL) + singleflight
                                   ├─ roteador de modelos (principal, fallback, shadow)
                                   ├─ provedores: DeepSeek | OpenAI-compatível | Falso (testes)
                                   │     cada um com timeout, retry e circuit breaker
                                   └─ registro de uso (SQLite, gravado em lote)
painel React ──▶ /admin/* (API do painel, protegida por senha/token de admin)
```

## Estrutura de pastas

```
cmd/comporta/          main mínimo: config por ambiente, liga dependências, graceful shutdown
internal/gateway/      domínio: Cliente, Requisicao, Uso, regras de orçamento, erros
internal/provedor/     interface Provedor + implementações (deepseek, openaicompat, falso)
internal/cache/        cache TTL + singleflight
internal/lgpd/         detecção e mascaramento de dados pessoais brasileiros (com validação de dígitos)
internal/resiliencia/  retry com backoff/jitter, circuit breaker
internal/armazenamento/ repositório SQLite (clientes, chaves com hash, uso)
internal/httpapi/      rotas públicas (/v1/...) e de admin (/admin/...), middlewares, SSE
web/                   painel React
carga/                 scripts k6
```

## API

- `POST /v1/chat/completions` — **compatível com o formato OpenAI** (assim qualquer SDK existente
  funciona só trocando a base URL). Suporta `stream: true`.
- `GET  /v1/models` — modelos liberados para aquele cliente.
- `GET  /healthz`, `GET /readyz`, `GET /metrics`.
- `/admin/clientes` (CRUD), `/admin/clientes/{id}/chaves` (gerar/revogar), `/admin/uso?de=&ate=&cliente=`.

Erros sempre no formato `{"erro": {"codigo": "...", "mensagem": "..."}}`, com status correto:
401 (chave inválida), 429 (rate limit, com `Retry-After`), 402 (orçamento esgotado),
502/503/504 (provedor com problema), 500 (interno, sem detalhes).

## Comandos

```bash
go run ./cmd/comporta                 # API em :8080 (usa o provedor falso se DEEPSEEK_API_KEY não existir)
go vet ./...
go test -race ./...                   # SEMPRE com -race
cd web && npm run dev                 # painel em :5173
k6 run carga/carga.js                 # contra o provedor falso
docker compose up
```

Variáveis: `PORT`, `DEEPSEEK_API_KEY`, `DEEPSEEK_BASE_URL`, `ADMIN_TOKEN`, `DB_PATH`.

## Regras de segurança (não negociáveis)

- A `DEEPSEEK_API_KEY` **nunca** vai para o front, para logs ou para o repositório. `.env` no `.gitignore`.
- Chaves de cliente guardadas só como **hash** (SHA-256); a chave inteira aparece uma única vez, na criação.
- Comparação de chaves/tokens com `crypto/subtle.ConstantTimeCompare`.
- Não logar o conteúdo das mensagens dos usuários por padrão (só tamanho, tokens, modelo). Tratar como dado pessoal (LGPD).
- Limite de tamanho do corpo da requisição.
- O mapa `[CPF_1] → valor real` vive só na memória da requisição; nunca é gravado em log, cache ou banco.
- Mascaramento acontece **antes** do cache: a chave de cache nunca contém dado pessoal.

## Mascaramento LGPD: regras

- CPF e CNPJ só contam como detectados se o **dígito verificador for válido** (evita mascarar
  números aleatórios). Aceitar com e sem pontuação.
- Telefone brasileiro com DDD (fixo e celular, com e sem +55), e-mail e cartão (com checagem de Luhn).
- O mesmo valor recebe sempre o mesmo marcador na mesma requisição (`[CPF_1]` aparece duas vezes
  se o CPF aparecer duas vezes), para o modelo manter a coerência.
- Restauração na resposta, inclusive no **streaming**: um marcador pode chegar quebrado entre dois
  pedaços (`[CP` + `F_1]`), então o restaurador precisa de um pequeno buffer.
- Testes table-driven com muitos casos reais e falsos positivos, mais um **fuzz test** garantindo
  que mascarar e restaurar devolve o texto original.
- Limitação honesta para o README: detecção por padrão não pega tudo (nomes, endereços). É uma
  camada de redução de risco, não garantia de conformidade.

## Convenções de Go

- `gofmt`; nada de imports ou variáveis não usados.
- Erros como valor: `fmt.Errorf("contexto: %w", err)`, `errors.Is`/`errors.As`, sem panic para erro esperado.
- Interfaces pequenas, definidas no pacote que as usa ("aceite interfaces, devolva structs").
- `context.Context` como primeiro parâmetro em tudo que faz I/O, e repassado adiante
  (se o cliente desconectar, a chamada ao provedor é cancelada).
- Um `http.Client` reaproveitado por provedor, com timeout; sempre fechar `resp.Body`.
- Estado compartilhado com mutex/atomic e coberto por teste concorrente com `-race`.
- Testes table-driven com `t.Run`; handlers com `httptest`; o provedor real é testado contra um
  `httptest.NewServer` que imita a API, nunca contra a API de verdade nos testes.
- Logs estruturados com `slog`.

## Como você deve trabalhar comigo (modo aprendizado)

- **Explique antes de escrever**: 2–3 frases sobre o que vai fazer e por quê, comparando com
  TypeScript/Node quando ajudar.
- **Me deixe escrever partes.** Quando a tarefa for um bom exercício (middleware, table-driven test,
  circuit breaker, singleflight), proponha a assinatura e os testes e me peça para implementar.
  Depois revise como num code review.
- Comentários no código em português, explicando o "porquê".
- Aponte quando algo funciona mas não é idiomático em Go.
- Ao fim de cada tarefa, sugira **uma pergunta de entrevista** sobre o que acabamos de fazer.
- Passos pequenos, um commit por passo, mensagens claras em português.
- Não invente como a Globo faz as coisas internamente. Só cite o que é público.

## Roadmap

**MVP até 13/10 (prioridade para a entrevista):**
- [ ] Provedor falso (latência e falhas configuráveis) + provedor DeepSeek, atrás da interface `Provedor`
- [ ] `POST /v1/chat/completions` compatível com OpenAI, sem e com streaming
- [ ] Chaves por cliente (hash no SQLite) + middleware de autenticação
- [ ] Rate limit por cliente (token bucket)
- [ ] Cache TTL + singleflight, com teste provando 1 chamada para N pedidos iguais simultâneos
- [ ] Timeout + retry com backoff/jitter + circuit breaker no provedor
- [ ] Registro de uso (tokens, custo estimado, latência, cache hit), gravado em lote
- [ ] Painel React: tabela de uso por cliente, cache hit rate, custo, latência p95, playground
- [ ] k6 contra o provedor falso: números no README (req/s, p95, quanto o cache economiza)
- [ ] README de portfólio com GIF, arquitetura, decisões e números

**Se sobrar tempo antes da entrevista (senão, primeiro item depois):**
- [ ] Mascaramento LGPD (sem streaming primeiro), com table-driven + fuzz test

**Depois da entrevista:**
- [ ] Restauração LGPD no streaming (buffer para marcadores quebrados)
- [ ] Orçamento mensal por cliente (402) e alertas
- [ ] Fallback entre modelos/provedores e modo shadow com relatório comparativo
- [ ] Métricas Prometheus + tracing OpenTelemetry
- [ ] Postgres + Redis (cache e rate limit compartilhados entre várias instâncias)
- [ ] Deploy no Cloud Run com Secret Manager
- [ ] Cache semântico (perguntas parecidas, não só idênticas), com cuidado e medição

## Decisões (registre aqui as novas, com o porquê)

- **API compatível com OpenAI:** adoção sem atrito; o cliente só troca a base URL.
- **Provedor falso desde o início:** testes e carga sem custo, sem rede e determinísticos.
- **Cache exato e opt-in** (header `X-Comporta-Cache` ou política do cliente): com raciocínio
  ligado por padrão, `temperature` não torna a resposta determinística, então reaproveitar
  precisa ser decisão explícita. O cache de prefixo da DeepSeek é preservado e medido.
- **Retry e fallback só antes do primeiro byte** enviado ao cliente.
- **Semáforo de concorrência por modelo** abaixo do limite da conta DeepSeek.
- **Uso gravado em lote:** não pôr uma escrita no banco no caminho crítico de cada requisição.
- **Mascarar antes de cachear:** o cache e os logs nunca veem dado pessoal.
- **Validar dígito verificador de CPF/CNPJ:** menos falsos positivos do que regex pura.

## Pitch de 30 segundos para a entrevista

"Construí em Go um gateway que deixa empresas brasileiras usarem IA sem vazar dado pessoal e com
custo sob controle. Ele mascara CPF, e-mail e telefone antes de enviar ao provedor e restaura na
volta. Cada aplicação tem sua chave, limite e orçamento; requisições iguais são cacheadas e deduplicadas com singleflight;
o provedor é protegido por timeout, retry e circuit breaker; e um modo shadow permite migrar de
modelo comparando os dois sem afetar o usuário. Medi com k6: [X] req/s com p95 de [Y] ms, e o
cache reduziu [Z]% das chamadas pagas. É o mesmo tipo de problema de um proxy que atende várias
aplicações clientes e precisa buscar dados externos de forma eficiente."
