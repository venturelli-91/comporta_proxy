# Log de execução

Uma linha por feature ou decisão. Mais recente embaixo.

| Data | Feature / evento | Commit | Notas |
|---|---|---|---|
| 09/10 | Spec inicial (CLAUDE.md + ARQUITETURA.md) | `5fd3867` | |
| 09/10 | Spec de design e plano v1 do MVP | `e7b18b3` | Escolha: um cliente HTTP compatível com OpenAI + falso no mesmo protocolo |
| 09/10 | 01 Config e custo | `6132d4c` | Preços em micro-dólares `int64`; desconto fora do pico |
| 09/10 | Plano v2 + softhouse em `.claude/` | — | Branch `feat/mvp`; execução Native; 8 papéis da softhouse adaptados |
| 09/10 | 02 | `feat(provedor)` | Cliente único compatível com OpenAI; falso no mesmo protocolo; ponytail: ResponseController.Flush |
