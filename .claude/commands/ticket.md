---
description: Implements a Comporta ticket by routing it to the right dev, then enforces independent code review (with ponytail) and QA, without merging or deploying.
---

Ticket: $ARGUMENTS

Run this chain, in order, with the Agent tool:

1. Pick the dev from the routing table in `.claude/softhouse.md` (`dev-backend` or `dev-frontend`). No clear scope → `tech-lead` first.
2. The dev delivers: an atomic commit, tests passing with `-race`, `.claude/docs/features/NN-*.md` and a line in `.claude/docs/log.md`.
3. `code-reviewer` (another agent, never the one who implemented it), with `ponytail:ponytail-review`. If there's a blocker, go back to step 2.
4. `qa-automation` validates the acceptance criteria. If it fails, go back to step 2.
5. If it touches keys, admin, LGPD or secrets → `security`; hot path → `performance`.
6. Stop. Tell the CEO what was done, the evidence and what's missing for approval. Push, merge and deploy only after his "yes".
