# 02 — Provedor (cliente compatível com OpenAI + falso)

**Pacote:** `internal/provedor` (`cliente.go`, `falso.go`)

## O que faz
- `Cliente.Enviar(ctx, corpo)` faz o `POST /chat/completions` em qualquer API compatível com OpenAI. Resposta 2xx volta como está, para o relay ler em stream. Qualquer outra vira `*ErroHTTP{Status, RetryAfter, Mensagem}`.
- `Falso` é um `http.Handler` que imita a DeepSeek: ecoa a última mensagem, manda `: keep-alive`, traz `usage` no último chunk e termina com `[DONE]`. Latência, velocidade por token e taxa de 503 são configuráveis.

## Por que assim (decisões)
- **Um cliente só:** DeepSeek e "openaicompat" diferem só na `base_url`. Uma interface com três implementações seria código a mais.
- **O falso fala HTTP de verdade:** testes e k6 passam pelo mesmo transporte, pool de conexões e parser do caminho real.
- **`Transport` próprio:** o padrão do Go guarda só 2 conexões ociosas por host, e sob carga isso vira handshake TLS o tempo todo.
- **Sem `http.Client.Timeout`:** ele mataria streams longos. Os prazos vêm do `context`.
- **`Repetivel()` e `Espera()` como métodos:** o pacote de retry enxerga só as interfaces e não importa o `provedor`.
- Ponytail aplicado: `http.NewResponseController(w).Flush()` no lugar da asserção manual de `http.Flusher`, e struct anônima no lugar de um tipo de uso único.

## Como testar
`go test ./internal/provedor/ -v`: sem stream, stream (keep-alive + usage + DONE), 503 repetível, 429 com Retry-After, 400 não repetível, rede fora do ar, contexto cancelado.

## Go vs TypeScript
- `context.Context` faz o papel do `AbortSignal` do `fetch`. Cancelar o contexto fecha a conexão com o provedor.
- `errors.As(err, &e)` corresponde ao `if (err instanceof ErroHTTP)`, só que funciona com erros embrulhados por `%w`.
- `httptest.NewServer` sobe um servidor real numa porta local, o que no Node exigiria `nock` ou `msw`.

## Exercício
Implemente o suporte a `Retry-After` em formato de data HTTP (`http.ParseTime`), com um caso novo na tabela do teste de 429.

## Pergunta de entrevista
"Por que o transporte HTTP padrão do Go é ruim para um proxy de alto volume, e o que você ajustou?"
