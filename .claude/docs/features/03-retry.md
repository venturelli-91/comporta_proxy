# 03 — Retry com backoff e jitter

**Pacote:** `internal/resiliencia` (`retry.go`)

## O que faz
`Retry(ctx, tentativas, base, fn)` chama `fn` até N vezes. Repete só erros que dizem `Repetivel() == true`
(429, 5xx, rede). A espera dobra a cada tentativa: metade fixa, metade aleatória. Um `Retry-After` do
provedor substitui o cálculo. Se a espera não cabe no prazo do contexto, desiste na hora.

## Por que assim (decisões)
- **Interfaces no consumidor:** `repetivel` e `comEspera` estão declaradas no pacote `resiliencia`. O `provedor.ErroHTTP` as satisfaz sem saber que elas existem (tipagem estrutural, como no TypeScript).
- **"Equal jitter" em vez de jitter cheio:** o piso de metade da espera impede tentativas quase imediatas, e o teste fica determinístico.
- **Desistir antes de estourar o prazo:** dormir até o contexto morrer só desperdiça uma goroutine e esconde o erro real.
- **Retry só antes do primeiro byte:** é quem chama (o roteador) que garante isso. O `Retry` não sabe nada de stream.

## Como testar
`go test ./internal/resiliencia/ -v`: sucesso na 1ª e na 3ª, tentativas esgotadas, erro não repetível, erro comum, Retry-After, contexto com prazo curto.

## Go vs TypeScript
- No Go, uma struct implementa uma interface sem declarar `implements`. É o mesmo duck typing estrutural do TS, só que verificado na compilação.
- `select` com `ctx.Done()` e um timer é o equivalente a `Promise.race([sleep(ms), abortPromise])`.

## Exercício
Acrescente um teto máximo de espera (por exemplo, 5 s) para que `base << i` não cresça sem limite, com um caso na tabela.

## Pergunta de entrevista
"Por que jitter? O que acontece com mil clientes fazendo retry exatamente no mesmo intervalo depois de uma queda?"
