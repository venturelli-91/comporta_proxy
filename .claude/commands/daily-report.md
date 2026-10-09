---
description: Gera o relatório diário da Comporta a partir do git log e de .claude/docs/log.md, com o tempo restante até a entrevista.
---

Monte o relatório abaixo usando `git log --since=midnight --oneline`, `.claude/docs/log.md` e `.claude/docs/backlog.md`.
Se houver aprendizado novo (falha recorrente, decisão, padrão que valeu a pena), registre em `.claude/docs/log.md`.

```
# Relatório diário — <data> (faltam N dias para 14/10)
## Entregue (commits)
## Em andamento
## Bloqueado / decisões pendentes do CEO
## Qualidade (testes, race, ponytail)
## Números (benchmark, k6)
## Custos (chamadas reais à DeepSeek, se houve)
## Aprendizados de Go do dia
## Próximas 3 ações
```
