---
description: Produces Comporta's daily report from git log and .claude/docs/log.md, with the time left until the interview.
---

Build the report below using `git log --since=midnight --oneline`, `.claude/docs/log.md` and `.claude/docs/backlog.md`.
If there is a new lesson (a recurring failure, a decision, a pattern that paid off), record it in `.claude/docs/log.md`.
Write the report in Portuguese (it's for Aurélio); anything recorded in the repo goes in English.

```
# Daily report — <date> (N days until 10/14)
## Delivered (commits)
## In progress
## Blocked / decisions pending from the CEO
## Quality (tests, race, ponytail)
## Numbers (benchmark, k6)
## Costs (real DeepSeek calls, if any)
## Go lessons of the day
## Next 3 actions
```
