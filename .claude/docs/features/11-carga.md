# 11 — Carga com k6

**Arquivo:** `carga/carga.js` · resultados brutos em `carga/resultados/` (fora do git)

## Como rodar (o mesmo jeito que gerou os números)
Gateway e k6 na **mesma rede Docker** (Linux), provedor falso com 60 ms fixos (a mesma latência do benchmark publicado pelo Bifrost):
```bash
docker compose build
docker network create carga
docker run -d --name gw --network carga -e ADMIN_TOKEN=adm -e FALSO_LATENCIA_MS=60 -e FALSO_POR_TOKEN_MS=0 -e FALSO_TAXA_ERRO=0 comporta_proxy-comporta
docker run --rm -i --network carga -e CENARIO=sem_cache -e RPS=1000 -e BASE=http://gw:8080 grafana/k6 run - < carga/carga.js
```
Para o `caos`, suba o `gw` com `FALSO_TAXA_ERRO=0.5`. O `setup()` do script cria o próprio cliente (com LGPD ligada) pelo admin.

## Números (09/10/2026, notebook Windows + Docker Desktop, 1.000 req/s por 30 s, LGPD ligada)
| Cenário | Vazão | p50 | p95 | p99 | Sucesso | Observação |
|---|---|---|---|---|---|---|
| `sem_cache` | 991 req/s | 60,8 ms | 61,7 ms | 64,7 ms | 100% | provedor = 60 ms → **overhead do gateway ≈ 0,8 ms (p50) e 1,7 ms (p95)** |
| `com_cache` (20 perguntas) | 999 req/s | 0,39 ms | 0,58 ms | 0,79 ms | 100% | **99,9% das chamadas pagas evitadas** |
| `caos` (provedor com 50% de 503) | 961 req/s | 63,9 ms | 649 ms | 977 ms | **98,4%** | teoria: 1 − (0,5³)² = 98,44% (3 tentativas × 2 destinos) |

Memória do container ao fim: ~22 MiB. Nenhum evento de uso descartado.

## Leitura honesta
- 1.000 req/s é a **taxa do teste**, não o limite do gateway (não procurei o ponto de saturação).
- 99,9% no `com_cache` vem de um cenário artificial com 20 perguntas repetidas. Na vida real, o ganho depende de quanto os clientes repetem perguntas.
- No `caos`, o p95 sobe para 649 ms por causa do backoff do retry. É o preço de transformar 50% de erro em 1,6%.
- Primeira tentativa com k6 no Docker Desktop apontando para o host Windows: 17 `connection refused` na rajada inicial (backlog de accept do host). O servidor respondeu 200 em todas as que chegaram. Por isso os números oficiais são todos dentro da rede Docker.

## Go vs TypeScript
O k6 é JavaScript, mas roda num runtime em Go (goja), não em Node: sem `npm`, sem `fetch`, com `http.post` síncrono por VU.

## Exercício
Ache o ponto de saturação: aumente `RPS` até o p99 passar de 100 ms no `sem_cache` e registre a vazão máxima com o `GOMAXPROCS` do container.

## Pergunta de entrevista
"Seu gateway transforma 50% de erro do provedor em 1,6%. De onde vem esse número e qual é o custo em latência?"
