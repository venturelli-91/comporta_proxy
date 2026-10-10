# Comporta

Gateway de LLM em Go. Fica entre as aplicações de uma empresa e os provedores de IA (DeepSeek ou qualquer
API compatível com OpenAI) e cuida do que costuma sair do controle: quem usa, quanto gasta, a mesma pergunta
paga duas vezes, o provedor que cai e o dado pessoal que vaza para fora do país.

![Painel da Comporta](docs/img/painel.png)

## Demo ao vivo

Uma instância pública roda na Render (plano gratuito), com o **provedor falso**: nada chega a uma IA de
verdade, então não há custo nem dado real.

1. Abra a demo e entre com o token **`demo-comporta`**.
2. Vá em **Playground**, cole a chave **`ck_demo_comporta`** e mande algo como
   `meu CPF é 529.982.247-25 e o e-mail ana@exemplo.com`.
3. A resposta volta com os dados originais, mas o provedor só viu `[CPF_1]` e `[EMAIL_1]`. A **Visão geral**
   mostra a requisição, o custo estimado e os dados pessoais mascarados.

Pela API, com qualquer SDK compatível com OpenAI:

```bash
curl https://<demo>/v1/chat/completions -H "Authorization: Bearer ck_demo_comporta" \
  -d '{"model":"rapido","messages":[{"role":"user","content":"meu CPF é 529.982.247-25"}]}'
```

Limites da demo: dorme após 15 min sem acesso (a primeira visita leva ~1 min), roda com 0,1 CPU (os números
abaixo foram medidos localmente) e o disco zera a cada reinício, recriando o cliente demo. O token de admin e a
chave são públicos de propósito: a instância é descartável e **nunca recebe a `DEEPSEEK_API_KEY`**
(ver `render.yaml`).

## O que faz

- **Uma chave por aplicação** (`ck_...`). A chave real do provedor fica só no servidor. No banco, só o SHA-256.
- **Limite e orçamento por cliente:** requisições por minuto (429 + `Retry-After`) e gasto mensal em US$ (402).
- **LGPD:** CPF e CNPJ (com dígito verificador), cartão (Luhn), e-mail e telefone com DDD são trocados por marcadores antes de sair (`meu CPF é [CPF_1]`) e restaurados na resposta, **inclusive no streaming**.
- **Cache exato opt-in + singleflight:** pedidos iguais e simultâneos viram uma única chamada paga.
- **Resiliência:** semáforo por modelo, retry com backoff e jitter (respeitando `Retry-After`), circuit breaker por destino e fallback para o próximo modelo, tudo antes do primeiro byte.
- **Streaming SSE** repassado token a token, com watchdog (keep-alive não conta como progresso) e cancelamento da chamada paga quando o cliente desconecta.
- **Modo shadow:** manda a mesma pergunta para um segundo modelo em segundo plano e compara latência, custo e tamanho. Serve para migrar de modelo sem risco.
- **Uso e custo** por cliente e por modelo (micro-dólares, com o preço fora do pico da DeepSeek), gravados em lote no SQLite.
- **Painel React** embutido no binário: vazão, gasto, economia do cache, p95, dados protegidos, clientes, chaves, shadow e playground.

## Números

k6 com 1.000 req/s por 30 s, gateway e k6 na mesma rede Docker, provedor falso com 60 ms fixos e LGPD ligada:

| Cenário | Vazão | p50 | p95 | p99 | Sucesso |
|---|---|---|---|---|---|
| Sem cache | 991 req/s | 60,8 ms | 61,7 ms | 64,7 ms | 100% |
| Com cache (20 perguntas repetidas) | 999 req/s | 0,39 ms | 0,58 ms | 0,79 ms | 100%, **99,9% das chamadas pagas evitadas** |
| Caos (provedor com 50% de erro 503) | 961 req/s | 63,9 ms | 649 ms | 977 ms | **98,4%** |

- **Overhead do gateway:** ~0,8 ms no p50 e ~1,7 ms no p95 sobre um provedor de 60 ms. Benchmark Go: ~0,19 ms por requisição, incluindo o salto HTTP até o falso (`BenchmarkOverhead`).
- **Caos:** 98,4% bate com a teoria, 1 − (0,5³)² (3 tentativas × 2 destinos). O custo é latência de backoff.
- **Imagem:** 24 MB (distroless). Memória em carga: ~22 MiB.

1.000 req/s é a taxa do teste, não o limite medido. O cenário com cache é artificial: na vida real o ganho depende de quanto as aplicações repetem perguntas. Metodologia em [`.claude/docs/features/11-carga.md`](.claude/docs/features/11-carga.md).

## Rodar

```bash
cp .env.example .env                 # defina ADMIN_TOKEN (o compose não sobe sem ele)
docker compose up --build            # http://localhost:8080, login com o ADMIN_TOKEN
# ou
go run ./cmd/comporta                 # sem DEEPSEEK_API_KEY usa o provedor falso; imprime um ADMIN_TOKEN se não houver
# atrás de outro proxy confiável (fora do Cloud Run): TRUST_PROXY=1 para o limite de login usar o X-Forwarded-For
```

Depois, no painel, cadastre um cliente, gere uma chave e use o Playground. Ou use qualquer SDK da OpenAI trocando só a base URL:

```bash
curl localhost:8080/v1/chat/completions -H "Authorization: Bearer ck_..." \
  -d '{"model":"rapido","stream":true,"messages":[{"role":"user","content":"meu CPF é 529.982.247-25"}]}'
```

Modelos (`rapido`, `forte`, `migracao`), preços, horário de pico, destinos de fallback e shadow ficam no
[`comporta.json`](comporta.json). Variáveis em [`.env.example`](.env.example).

## Arquitetura

```
apps clientes ──(Bearer ck_...)──▶ Comporta (Go, net/http)
   auth (hash → snapshot atômico) → rate limit → orçamento → limite de corpo
   → LGPD (mascara) → cache opt-in + singleflight │ shadow em segundo plano
   → roteador: semáforo → circuit breaker → retry → fallback
   → relay JSON/SSE (watchdog, restaura PII, captura usage) → custo → uso em lote (SQLite)
painel React (go:embed) ──▶ /admin/* (cookie HttpOnly)
```

| Pacote | Papel |
|---|---|
| `internal/provedor` | Cliente HTTP compatível com OpenAI + provedor falso no mesmo protocolo |
| `internal/gateway` | Config, custo com horário de pico, roteador |
| `internal/resiliencia` | Retry com backoff, jitter e `Retry-After` |
| `internal/cache` | LRU com TTL e limite em bytes |
| `internal/lgpd` | Mascaramento e restauração (inclusive em stream) |
| `internal/armazenamento` | SQLite: clientes, chaves (hash), uso em lote, relatórios |
| `internal/httpapi` | Rotas, auth, admissão, chat, relay, admin, métricas |
| `web/` | Painel React |

Decisões e trade-offs: [`docs/ARQUITETURA.md`](docs/ARQUITETURA.md) e a seção "Decisões" do [`CLAUDE.md`](CLAUDE.md).
Uma página por feature, com o porquê de cada escolha: [`.claude/docs/features/`](.claude/docs/features/).

## Testes

```bash
go test ./...
docker run --rm -v "$PWD:/src" -w /src golang:1.27 go test -race ./...   # race detector
go test ./internal/lgpd/ -run x -fuzz FuzzMascararRestaurar -fuzztime 30s
go test -run x -bench Overhead -benchmem ./internal/httpapi/
```

Os testes de integração sobem o gateway e o provedor falso em `httptest` e cobrem 401, 429, 402, 403, 413,
stream, cache, singleflight (10 pedidos → 1 chamada), LGPD ida e volta, fallback, revogação imediata de chave,
shadow, cliente que desconecta (499) e provedor que só manda keep-alive (504). Dois fuzz tests provam que
mascarar e restaurar devolve o texto original, mesmo com marcadores partidos entre pedaços do stream.

## Limitações

- **LGPD:** a detecção por padrão não pega nomes, endereços nem dados sem formato fixo. É uma camada de redução de risco, não garantia de conformidade.
- **Uma instância:** cache, rate limit e orçamento vivem em memória. Com várias instâncias (Cloud Run escalando), precisam ir para o Redis.
- **Preços** da DeepSeek no `comporta.json` são exemplos. Confira a tabela oficial antes de usar com dinheiro real.
- Dívida deliberada, com o gatilho para pagar cada item: [`.claude/docs/backlog.md`](.claude/docs/backlog.md).

## Como foi feito

O processo (softhouse de agentes, checklists, log por feature, comunicação para cada público) está em
[`.claude/`](.claude/README.md). Cada feature é um commit, com teste, revisão de simplicidade (ponytail) e uma página de documentação.
