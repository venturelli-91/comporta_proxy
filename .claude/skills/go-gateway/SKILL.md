---
name: go-gateway
description: Convenções Go da Comporta e o checklist de cada feature. Use antes de escrever ou revisar qualquer código em internal/ ou cmd/ deste repositório.
---

# Go na Comporta

A fonte da verdade é o `CLAUDE.md` (Convenções de Go, Regras de segurança). Este resumo serve para consulta rápida.

## Ambiente (Windows)
```powershell
$env:PATH="C:\Users\PC\sdk\go1.27.2\bin;$env:PATH"
go vet ./...; gofmt -l .
go test ./...
# -race precisa de cgo/gcc: roda no Docker
docker run --rm -v "${PWD}:/src" -v comporta-gomod:/go/pkg/mod -w /src golang:1.27 go test -race ./...
```

## Padrões do repositório
- Erros do provedor: `*provedor.ErroHTTP` com `Repetivel()` e `Espera()`. O retry (`resiliencia.Retry`) usa essas duas interfaces e não depende do pacote provedor.
- Dinheiro em micro-dólares `int64` (`gateway.Config.Custo`).
- Testes de rede: `httptest.NewServer(&provedor.Falso{...})`. `Falso.Chamadas` conta as chamadas e serve para provar o singleflight.
- Estado lido em toda requisição (clientes, chaves): snapshot em `atomic.Pointer`, trocado inteiro.
- Erro ao cliente: `{"erro":{"codigo","mensagem"}}` com o status da tabela do `CLAUDE.md`.

## Idiomas que um dev TS costuma errar
- `for` com goroutine: desde o Go 1.22 cada iteração tem a própria variável. Não precisa mais de `v := v`.
- Interface pequena definida no pacote que a consome, não no que a implementa.
- `defer resp.Body.Close()` só depois de checar `err`.
- `context` é o "AbortController" do Go: passe adiante, não guarde em struct.
- Mapa não é seguro para concorrência: mutex, `sync.Map` ou snapshot imutável.

## Checklist de feature
Ver `.claude/docs/checklist.md`.
