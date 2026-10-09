# 01 — Config e custo

**Commit:** `6132d4c` · **Pacote:** `internal/gateway` (`config.go`, `custo.go`)

## O que faz
Lê o `comporta.json` (provedores, aliases de modelo, preços, janelas de pico, cache, timeouts) e calcula o
custo de uma chamada em micro-dólares a partir do bloco `usage`, aplicando o desconto fora do pico.

## Por que assim (decisões)
- **Aliases** (`rapido`, `forte`, `migracao`): o cliente nunca vê o nome real do modelo, então trocar de provedor não exige mudar nenhuma aplicação.
- **Micro-dólares em `int64`**: `float` acumula erro de arredondamento em somas de milhões de chamadas. O float só existe na leitura da config.
- **Soma antes de dividir**: em chamadas pequenas, dividir token a token zeraria o custo.
- **Modelo sem preço custa 0**: registrar a chamada sem custo é melhor do que derrubar a requisição por falta de config.

## Como testar
`go test ./internal/gateway/ -run TestCusto -v` (pico, fora do pico, fim de semana, cache de prefixo, modelo sem preço).

## Go vs TypeScript
- Tags de struct (`json:"base_url"`) fazem o papel do schema do Zod, mas sem validação: por isso existe `validar()`.
- `time.Time.UTC().Weekday()` substitui `date-fns`; não há dependência para datas.

## Exercício
Acrescente uma segunda janela de preço (por exemplo, a madrugada com 75% de desconto) sem quebrar os testes que já existem.

## Pergunta de entrevista
"Por que você não usou `float64` para dinheiro, e onde o arredondamento acontece?"
