# Softhouse — regras aplicadas à Comporta

Adaptado do framework `venturelli-house/CLAUDE.md`. O CEO é o Aurélio. Produto, stack e convenções ficam no
`CLAUDE.md` da raiz; aqui ficam só o processo e os guardrails.

## Princípios
1. Não começar sem objetivo, usuário e critério de aceite (spec em `docs/superpowers/specs/`).
2. Tickets pequenos, independentes e verificáveis: uma feature = um commit.
3. Ninguém aprova a própria entrega: todo diff passa por `code-reviewer` (com ponytail) e `qa-automation`.
4. Merge na main, deploy, nova dependência, mudança de API pública ou de schema: só com aprovação explícita do CEO.
5. Toda entrega tem evidência: saída de teste, benchmark, k6, screenshot.
6. Ambiguidade relevante: parar e perguntar ao CEO.
7. No máximo 5 tickets ativos.
8. Prioridade: **P0** (segurança, bloqueio, prazo da entrevista), **P1** (semana), **P2** (melhoria), **Bug**.
9. Registrar decisões em `CLAUDE.md` / `docs/ARQUITETURA.md`, e aprendizados em `.claude/docs/log.md`.

## Fluxo
```
Pedido do CEO → [claro] → dev-backend | dev-frontend → code-reviewer (ponytail) → qa-automation
             → [ambíguo] → tech-lead (ticket com contrato) → dev → code-reviewer → qa
             → seguranca / performance quando o ticket toca auth, LGPD, caminho quente
             → aprovação do CEO → devops-sre (só se houver deploy)
```
Não force a cadeia inteira: `seguranca` e `performance` só entram quando o ticket toca a área deles.

## Roteamento
| Trabalho | Agente |
|---|---|
| Go: API, provedor, cache, LGPD, banco, resiliência | `dev-backend` |
| Painel React, playground | `dev-frontend` |
| Escopo, contrato, trade-off de arquitetura | `tech-lead` |
| Todo diff | `code-reviewer` → `qa-automation` |
| Chaves, admin, PII, segredos | `seguranca` |
| Latência, alocação, k6, números | `performance` |
| Docker, compose, Cloud Run | `devops-sre` |

Papéis do framework que ficaram de fora (mobile, dados, evals, prompt, integrações, triagem, PM,
watchdog, gestor de conhecimento): não há trabalho deles neste projeto. Se surgir, copie de `venturelli-house/.claude/agents/`.

## Matriz de autonomia
- **Sem aprovação:** ler código e docs, criar branch, implementar, rodar testes, commitar no branch, gerar relatórios.
- **Com aprovação:** push, abrir PR, merge, deploy, dependência nova, mudança de API pública ou schema, gasto com a API real da DeepSeek.
- **Nunca sem instrução explícita:** produção, exclusão de dados, rotação ou exposição de credenciais.

Na sessão de 09/10/2026 o CEO autorizou push do branch `feat/mvp` e abertura de PR no fim. O merge continua com ele.

## Ponytail em todo PR
`code-reviewer` roda `ponytail:ponytail-review` em todo diff. Ao fim de um marco, `ponytail:ponytail-audit`
no repo inteiro e `ponytail:ponytail-debt` para manter a dívida deliberada (`// ponytail:`) no backlog.
A régua é a fronteira de confiança: validar entrada HTTP, resposta do provedor e dado do banco é
obrigatório; revalidar o que o próprio tipo garante é achado.

## Commits
Conventional Commits em português com escopo (`feat(cache): ...`), um por feature, com corpo explicando
o porquê e a linha `Co-Authored-By` quando gerado com o Claude.
