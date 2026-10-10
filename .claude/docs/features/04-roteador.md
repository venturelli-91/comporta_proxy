# 04 — Roteador (semáforo, circuit breaker, retry, fallback)

**Pacote:** `internal/gateway` (`roteador.go`)

## O que faz
`Chamar(ctx, alias, corpo)` leva um alias (`rapido`) até um provedor:
1. **Semáforo do alias** (canal com buffer do tamanho de `concorrencia`). Se não abrir vaga em 2 s → `ErrOcupado` (429).
2. Para cada destino em ordem: **circuit breaker** (`gobreaker/v2`) → **retry** (3 tentativas, Task 3) → `Cliente.Enviar`.
3. Se o destino falha por culpa do provedor (5xx, 429, rede, breaker aberto), passa para o próximo. Erro do cliente (400/422) para na hora.
4. A vaga no semáforo é devolvida quando o corpo da resposta é fechado, ou seja, só no fim do stream.

`ChamarDestino` chama um destino específico e é reutilizado pelo modo shadow. `Estado()` mostra os breakers no painel.

## Por que assim (decisões)
- **Retry dentro do breaker:** o breaker conta o resultado final. Três tentativas que falham contam como uma falha, não três.
- **Breaker abre com ≥ 5 chamadas e ≥ 50% de falhas:** uma falha isolada no começo não derruba o destino.
- **4xx do cliente conta como sucesso** e **cancelamento é excluído**: nenhum dos dois diz que o provedor está doente.
- **A vaga do semáforo é liberada no `Close` do corpo** (`sync.OnceFunc`): um stream de 2 minutos ocupa a vaga pelos 2 minutos, que é o que o limite da DeepSeek mede.
- Ponytail: a lista "destinos + shadow" virou `ModeloCfg.todos()`, usada pela validação e pelo roteador. A ocupação do semáforo no `Estado()` ficou para quando houver métricas Prometheus.

## Como testar
`go test ./internal/gateway/ -run 'Fallback|Erro|Breaker|SemDestino|Semaforo' -v`

## Go vs TypeScript
- Canal com buffer como semáforo: `sem <- struct{}{}` ocupa e `<-sem` libera. No Node seria um `p-limit`.
- `select` com três casos (vaga, timeout, cancelamento) não tem equivalente direto em `Promise`. É o recurso mais "Go" do arquivo.
- Embutir `io.ReadCloser` na struct `corpoComFim` e sobrescrever só o `Close` é composição, o "extends" do Go.

## Exercício
Implemente o limite de concorrência **por cliente** (além do limite por alias), com um teste mostrando que um cliente não consegue ocupar todas as vagas.

## Pergunta de entrevista
"Onde você colocou o retry em relação ao circuit breaker, e o que mudaria se fosse o contrário?"
