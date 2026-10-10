---
description: Roda o ciclo completo de uma feature da Comporta (Definition of Done) — TDD, vet, race, ponytail-review, documentação e commit atômico.
---

Feature: $ARGUMENTS

Siga `.claude/docs/checklist.md`, seção "Definition of Done por feature", item por item:

1. Releia a seção correspondente em `docs/ARQUITETURA.md` e na spec.
2. TDD: teste falhando (veja falhar), código mínimo, teste passando.
3. `gofmt -l .` e `go vet ./...` sem saída; `go test -race ./...` verde no Docker.
4. Skill `ponytail:ponytail-review` no diff. Aplique os cortes; o que ficar como atalho deliberado leva `// ponytail:`.
5. Crie `.claude/docs/features/NN-<nome>.md` a partir do modelo em `.claude/docs/features/README.md`.
6. Acrescente uma linha em `.claude/docs/log.md` e marque o item em `.claude/docs/backlog.md`.
7. Um commit com código + docs: `feat(<escopo>): <descrição>` em português.
