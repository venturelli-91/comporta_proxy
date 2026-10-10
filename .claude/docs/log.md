# Log de execução

Uma linha por feature ou decisão. Mais recente embaixo.

| Data | Feature / evento | Commit | Notas |
|---|---|---|---|
| 09/10 | Spec inicial (CLAUDE.md + ARQUITETURA.md) | `5fd3867` | |
| 09/10 | Spec de design e plano v1 do MVP | `e7b18b3` | Escolha: um cliente HTTP compatível com OpenAI + falso no mesmo protocolo |
| 09/10 | 01 Config e custo | `6132d4c` | Preços em micro-dólares `int64`; desconto fora do pico |
| 09/10 | Plano v2 + softhouse em `.claude/` | — | Branch `feat/mvp`; execução Native; 8 papéis da softhouse adaptados |
| 09/10 | 02 | `feat(provedor)` | Cliente único compatível com OpenAI; falso no mesmo protocolo; ponytail: ResponseController.Flush |
| 09/10 | 03 | `feat(resiliencia)` | Equal jitter (piso de metade); Retry-After vence; desiste se não cabe no prazo |
| 09/10 | 04 | `feat(gateway)` | Retry dentro do breaker; 4xx do cliente não abre o breaker; vaga do semáforo liberada no Close do corpo |
| 09/10 | 05 | `feat(cache)` | LRU por bytes com container/list; expiração preguiçosa; mutex global (ponytail) |
| 09/10 | 06 | `feat(lgpd)` | DV de CPF/CNPJ e Luhn; marcador nunca colide com texto do usuário; fuzz 500k+3,2M execs ok |
| 09/10 | 07 | `feat(armazenamento)` | Chave rand.Text 128 bits só como SHA-256; lote 500/1s drenado no shutdown; p95 por OFFSET |
| 09/10 | 08 | `feat(httpapi)` | Bench overhead ~0,19 ms/req (loopback); singleflight 10→1; 499 e 504 testados; mutantes LGPD/singleflight mortos |
| 09/10 | 09 | `feat` | Imagem distroless 23,5 MB; smoke curl ok; SIGTERM drena uso |
| 09/10 | 10 | `feat(web)` | Barra de vazão como elemento central; bug do TanStack v5 (query undefined) pego no screenshot; imagem 24,3 MB com painel |
| 09/10 | 11 | `test(carga)` | 991 req/s p95 61,7 ms (falso 60 ms) → overhead ~1,7 ms; cache evita 99,9%; caos 50% → 98,4% sucesso |
| 09/10 | Auditoria ponytail (repo inteiro) | `refactor` | 1 corte: campo/parâmetro `nome` morto em `provedor.Novo`; resto enxuto, dívida já marcada com `ponytail:` |
| 09/10 | 12 Docs finais | `docs` | README com números; CLAUDE.md (roadmap, decisões, pitch preenchido); ARQUITETURA.md (estado atual); comunicação atualizada |
| 09/10 | Correções da revisão final | `fix(httpapi)` | 7 achados importantes corrigidos com teste RED→GREEN; relay sem goroutine (vigia); 104 testes; race 2x ok |
| 09/10 | Segurança: ID reaproveitado | `fix(armazenamento)` | AUTOINCREMENT em clientes e chaves; bancos antigos precisam ser recriados (MVP sem dados de produção) |
| 09/10 | Segurança: limite simultâneo por cliente | `fix(httpapi)` | 32 requisições abertas por chave (abaixo da concorrência do alias); acima disso 429 concorrencia_excedida |
