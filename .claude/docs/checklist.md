# Checklists

## Definition of Done por feature
- [ ] Reli a seção de `docs/ARQUITETURA.md` e da spec que a feature toca
- [ ] Vi o teste falhar antes de implementar
- [ ] `gofmt -l .` e `go vet ./...` sem saída
- [ ] `go test -race ./...` verde (Docker)
- [ ] `ponytail:ponytail-review` no diff; cortes aplicados, atalhos marcados com `// ponytail:`
- [ ] Nada de segredo, chave inteira ou conteúdo de mensagem em log
- [ ] `.claude/docs/features/NN-*.md` criado
- [ ] Linha em `.claude/docs/log.md`; item marcado em `.claude/docs/backlog.md`
- [ ] Um commit atômico, Conventional Commits em português

## Antes do PR
- [ ] `ponytail:ponytail-audit` no repo e `ponytail:ponytail-debt` → backlog
- [ ] Review independente do branch inteiro
- [ ] Smoke: `go run ./cmd/comporta` + `curl` com e sem stream; painel abre
- [ ] README com números reais (benchmark e k6)
- [ ] `.env` fora do git; `grep -ri "sk-" .` sem segredo real

## Antes da entrevista (13/10)
- [ ] Demo de 3 minutos ensaiada: chave → chat → stream → CPF mascarado → cache hit → painel
- [ ] Pitch com os números [X] req/s, p95 [Y] ms e [Z]% de economia preenchidos
- [ ] Rodar a skill `entrevista` pelo menos uma vez
- [ ] Saber explicar: singleflight, circuit breaker, context/cancelamento, `atomic.Pointer`, gravação em lote
