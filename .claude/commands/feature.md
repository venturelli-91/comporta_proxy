---
description: Runs the full cycle of a Comporta feature (Definition of Done) — TDD, vet, race, ponytail-review, documentation and an atomic commit.
---

Feature: $ARGUMENTS

Follow `.claude/docs/checklist.md`, section "Definition of Done per feature", item by item:

1. Re-read the matching section in `docs/ARCHITECTURE.md` and in the spec.
2. TDD: a failing test (watch it fail), minimal code, a passing test.
3. `gofmt -l .` and `go vet ./...` with no output; `go test -race ./...` green in Docker.
4. The `ponytail:ponytail-review` skill on the diff. Apply the cuts; whatever stays as a deliberate shortcut gets `// ponytail:`.
5. Create `.claude/docs/features/NN-<name>.md` from the template in `.claude/docs/features/README.md`.
6. Add a line to `.claude/docs/log.md` and tick the item in `.claude/docs/backlog.md`.
7. One commit with code + docs: `feat(<scope>): <description>` in English.
