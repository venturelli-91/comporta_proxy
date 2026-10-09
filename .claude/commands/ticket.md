---
description: Implementa um ticket da Comporta roteando para o dev certo, depois força code review (com ponytail) e QA independentes, sem merge nem deploy.
---

Ticket: $ARGUMENTS

Rode esta cadeia, em ordem, com o Agent tool:

1. Escolha o dev pela tabela de roteamento em `.claude/softhouse.md` (`dev-backend` ou `dev-frontend`). Sem escopo claro → `tech-lead` primeiro.
2. O dev entrega: commit atômico, testes passando com `-race`, `.claude/docs/features/NN-*.md` e uma linha em `.claude/docs/log.md`.
3. `code-reviewer` (outro agente, nunca o mesmo que implementou), com `ponytail:ponytail-review`. Se houver bloqueador, volte ao passo 2.
4. `qa-automation` valida os critérios de aceite. Se reprovar, volte ao passo 2.
5. Se tocar chaves, admin, LGPD ou segredos → `seguranca`; caminho quente → `performance`.
6. Pare. Liste ao CEO o que foi feito, as evidências e o que falta para aprovar. Push, merge e deploy só com o "sim" dele.
