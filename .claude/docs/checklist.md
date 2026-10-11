# Checklists

## Definition of Done per feature
- [ ] Re-read the section of `docs/ARCHITECTURE.md` and of the spec that the feature touches
- [ ] Watched the test fail before implementing
- [ ] `gofmt -l .` and `go vet ./...` with no output
- [ ] `go test -race ./...` green (Docker)
- [ ] `ponytail:ponytail-review` on the diff; cuts applied, shortcuts marked with `// ponytail:`
- [ ] No secrets, full keys or message content in logs
- [ ] `.claude/docs/features/NN-*.md` created
- [ ] A line in `.claude/docs/log.md`; item ticked in `.claude/docs/backlog.md`
- [ ] One atomic commit, Conventional Commits in English

## Before the PR
- [ ] `ponytail:ponytail-audit` on the repo and `ponytail:ponytail-debt` → backlog
- [ ] Independent review of the whole branch
- [ ] Smoke: `go run ./cmd/comporta` + `curl` with and without streaming; the dashboard opens
- [ ] README with real numbers (benchmark and k6)
- [ ] `.env` out of git; `grep -ri "sk-" .` finds no real secret

## Before the interview (10/13)
- [ ] 3-minute demo rehearsed: key → chat → stream → masked CPF → cache hit → dashboard
- [ ] Pitch with the numbers filled in (req/s, p95, % saved)
- [ ] Run the `interview` skill at least once
- [ ] Be able to explain: singleflight, circuit breaker, context/cancellation, `atomic.Pointer`, batched writes
