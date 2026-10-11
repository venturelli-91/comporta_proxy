# Software house — rules applied to Comporta

Adapted from the `venturelli-house/CLAUDE.md` framework. The CEO is Aurélio. Product, stack and conventions live
in the root `CLAUDE.md`; this file holds only the process and the guardrails.

## Principles
1. Don't start without a goal, a user and an acceptance criterion (spec in `docs/superpowers/specs/`).
2. Small, independent, verifiable tickets: one feature = one commit.
3. Nobody approves their own work: every diff goes through `code-reviewer` (with ponytail) and `qa-automation`.
4. Merging to main, deploying, a new dependency, a change to the public API or the schema: only with the CEO's explicit approval.
5. Every delivery has evidence: test output, benchmark, k6, screenshot.
6. Relevant ambiguity: stop and ask the CEO.
7. At most 5 active tickets.
8. Priority: **P0** (security, blocker, interview deadline), **P1** (this week), **P2** (improvement), **Bug**.
9. Record decisions in `CLAUDE.md` / `docs/ARCHITECTURE.md`, and lessons in `.claude/docs/log.md`.

## Flow
```
CEO request → [clear] → dev-backend | dev-frontend → code-reviewer (ponytail) → qa-automation
            → [ambiguous] → tech-lead (ticket with a contract) → dev → code-reviewer → qa
            → security / performance when the ticket touches auth, LGPD, the hot path
            → CEO approval → devops-sre (only if there is a deploy)
```
Don't force the whole chain: `security` and `performance` only join when the ticket touches their area.

## Routing
| Work | Agent |
|---|---|
| Go: API, provider, cache, LGPD, database, resilience | `dev-backend` |
| React dashboard, playground | `dev-frontend` |
| Scope, contract, architecture trade-off | `tech-lead` |
| Every diff | `code-reviewer` → `qa-automation` |
| Keys, admin, PII, secrets | `security` |
| Latency, allocation, k6, numbers | `performance` |
| Docker, compose, Cloud Run | `devops-sre` |

Framework roles left out (mobile, data, evals, prompt, integrations, triage, PM, watchdog, knowledge
manager): there is no work for them in this project. If it shows up, copy from `venturelli-house/.claude/agents/`.

## Autonomy matrix
- **No approval needed:** read code and docs, create a branch, implement, run tests, commit on the branch, produce reports.
- **Approval needed:** push, open a PR, merge, deploy, new dependency, public API or schema change, spending on the real DeepSeek API.
- **Never without explicit instruction:** production, deleting data, rotating or exposing credentials.

In the 2026-10-09 session the CEO authorized pushing the `feat/mvp` branch and opening a PR at the end. Merging stays with him.

## Ponytail on every PR
`code-reviewer` runs `ponytail:ponytail-review` on every diff. At the end of a milestone, `ponytail:ponytail-audit`
on the whole repo and `ponytail:ponytail-debt` to keep the deliberate debt (`// ponytail:`) in the backlog.
The rule of thumb is the trust boundary: validating HTTP input, provider responses and database data is
mandatory; re-validating what the type already guarantees is a finding.

## Commits
Conventional Commits in English with a scope (`feat(cache): ...`), one per feature, with a body explaining
the why and the `Co-Authored-By` line when generated with Claude.
