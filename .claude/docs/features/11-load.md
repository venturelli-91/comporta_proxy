# 11 — Load testing with k6

**File:** `carga/carga.js` · raw results in `carga/resultados/` (outside git)

## How to run (the same way that produced the numbers)
Gateway and k6 on the **same Docker network** (Linux), fake provider with a fixed 60 ms (the same latency as the benchmark Bifrost publishes):
```bash
docker compose build
docker network create carga
docker run -d --name gw --network carga -e ADMIN_TOKEN=adm -e FALSO_LATENCIA_MS=60 -e FALSO_POR_TOKEN_MS=0 -e FALSO_TAXA_ERRO=0 comporta_proxy-comporta
docker run --rm -i --network carga -e CENARIO=sem_cache -e RPS=1000 -e BASE=http://gw:8080 grafana/k6 run - < carga/carga.js
```
For `caos` (chaos), start `gw` with `FALSO_TAXA_ERRO=0.5`. The script's `setup()` creates its own client (with LGPD on) through the admin.

## Numbers (2026-10-09, Windows laptop + Docker Desktop, 1,000 req/s for 30 s, LGPD on)
| Scenario | Throughput | p50 | p95 | p99 | Success | Note |
|---|---|---|---|---|---|---|
| `sem_cache` (no cache) | 991 req/s | 60.8 ms | 61.7 ms | 64.7 ms | 100% | provider = 60 ms → **gateway overhead ≈ 0.8 ms (p50) and 1.7 ms (p95)** |
| `com_cache` (cache, 20 questions) | 999 req/s | 0.39 ms | 0.58 ms | 0.79 ms | 100% | **99.9% of paid calls avoided** |
| `caos` (provider with 50% 503s) | 961 req/s | 63.9 ms | 649 ms | 977 ms | **98.4%** | theory: 1 − (0.5³)² = 98.44% (3 attempts × 2 destinations) |

Container memory at the end: ~22 MiB. No usage events dropped.

## Honest reading
- 1,000 req/s is the **test rate**, not the gateway's limit (I didn't look for the saturation point).
- The 99.9% in `com_cache` comes from an artificial scenario with 20 repeated questions. In real life the gain depends on how often clients repeat questions.
- In `caos`, p95 rises to 649 ms because of the retry backoff. It's the price of turning 50% errors into 1.6%.
- First attempt with k6 on Docker Desktop pointing at the Windows host: 17 `connection refused` in the initial burst (the host's accept backlog). The server answered 200 to every request that reached it. That's why the official numbers are all inside the Docker network.

## Go vs TypeScript
k6 is JavaScript, but it runs on a Go runtime (goja), not on Node: no `npm`, no `fetch`, with a synchronous `http.post` per VU.

## Exercise
Find the saturation point: raise `RPS` until p99 goes above 100 ms in `sem_cache` and record the maximum throughput with the container's `GOMAXPROCS`.

## Interview question
"Your gateway turns 50% provider errors into 1.6%. Where does that number come from, and what does it cost in latency?"
