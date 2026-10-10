# Comporta — para tech leads

## Escopo
Gateway de LLM em Go (stdlib `net/http`), API compatível com OpenAI, uma instância, SQLite. Painel React embutido no binário.

## Decisões e trade-offs
| Decisão | Ganho | Custo / risco |
|---|---|---|
| Um cliente HTTP compatível com OpenAI em vez de uma interface com N provedores | Menos código; o falso exercita o mesmo caminho de rede | Provedor não compatível exigiria um adaptador |
| Cache exato **opt-in** | Não devolve resposta "congelada" de modelo com raciocínio | Menos hits por padrão |
| Retry e fallback **só antes do primeiro byte** | Nunca duplica conteúdo no stream | Falha no meio do stream vira erro para o cliente |
| Estado em memória (cache, rate limit, orçamento) | Simples e rápido | Com várias instâncias deixa de ser global: o gatilho para Redis |
| Uso gravado em lote, descartado se o canal encher | A API nunca espera o banco | Pode perder registros sob pico extremo (contado na métrica) |
| Mascaramento por padrão + dígito verificador | Poucos falsos positivos, determinístico (preserva o cache de prefixo) | Não pega nomes nem endereços |

## Riscos
- Preços e nomes de modelo da DeepSeek mudam: estão na config, mas precisam ser conferidos antes de uso real.
- Singleflight com stream: os seguidores recebem a resposta inteira de uma vez, não ao vivo (fanout fica no backlog).
- Escalar horizontalmente exige Redis para o rate limit, o orçamento e o cache.

## Resultado (09/10)
12 commits por feature no branch `feat/mvp`. `go test -race ./...` verde, 2 fuzz tests, testes de integração para
todos os códigos de erro, k6 em 3 cenários (README). Overhead ~1,7 ms no p95; caos com 50% de 503 → 98,4% de sucesso.

## Segurança (09/10, fim da sessão)
Corrigido e publicado no PR #1:
- IDs não reaproveitados.
- Limite de 32 requisições simultâneas por cliente.
- Breaker por cliente e destino.
- Fallback em 401/402/403 do provedor.
- Config exige preço.
- Sessão de admin revogável.

Em aberto (topo do `.claude/docs/backlog.md`): 3 falhas no limite de tentativas de login (corrida, `X-Forwarded-For` forjável sem proxy, reset do mapa). O risco é moderado: o token do admin gerado pelo binário tem 128 bits.

## Como validar
`go test -race ./...`, `BenchmarkOverhead` e os três cenários do k6 em `carga/`. Os números vão para o README.
