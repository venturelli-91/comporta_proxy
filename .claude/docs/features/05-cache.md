# 05 — Cache LRU com TTL

**Pacote:** `internal/cache` (`cache.go`)

## O que faz
Guarda respostas inteiras (JSON, ou o stream SSE completo para replay) por chave, com TTL e limite em
**bytes**. Quando enche, tira o menos usado recentemente. Cada entrada guarda o custo original, que vira
"economia" a cada hit.

## Por que assim (decisões)
- **`container/list` + mapa**, da stdlib: a lista dá a ordem de uso em O(1) e o mapa dá a busca em O(1). Sem dependência.
- **Limite em bytes:** respostas de LLM variam de 100 B a centenas de KB, então contar itens não diz nada sobre a memória.
- **`Lock` no `Get`:** um hit mexe na lista, então `RWMutex` não ajudaria.
- **Expiração preguiçosa:** a entrada vencida é removida quando alguém a pede ou quando o LRU a despeja. Não há goroutine de limpeza.
- Ponytail: mutex global marcado no código, com o caminho de upgrade (shards por hash) caso apareça disputa no profile.
- O cache é **opt-in** e a chave é calculada **depois do mascaramento LGPD** (Task 8): o cache nunca vê dado pessoal.

## Como testar
`go test ./internal/cache/ -v`: hit, TTL, despejo LRU, entrada maior que o cache, substituição, concorrência (`-race`).

## Go vs TypeScript
- Um `Map` do JS já preserva a ordem de inserção, e o LRU clássico em JS usa `delete` + `set`. No Go o mapa não tem ordem, por isso existe a lista.
- `el.Value.(*item)` é uma *type assertion*: a lista guarda `any`, como um `unknown` que você converte.

## Exercício
Troque o mutex global por 16 shards (`fnv` da chave → índice) e meça com `go test -bench` antes e depois.

## Pergunta de entrevista
"Por que o seu LRU limita bytes e não itens, e o que acontece com uma resposta maior que o cache inteiro?"
