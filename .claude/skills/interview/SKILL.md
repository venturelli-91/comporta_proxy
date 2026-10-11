---
name: interview
description: Prepares Aurélio for the 10/14 interview (a Go proxy on GCP) based on the features already built. Use when he asks "me treina", "simulado", "perguntas de entrevista", "revisar o pitch", "train me", "mock interview" or "interview questions".
---

# Comporta interview practice

The interview is in Portuguese: run the practice in Portuguese.

1. Read `.claude/docs/features/*.md` (the "Interview question" and "Decisions" sections) and the pitch in `CLAUDE.md`.
2. Ask **one question at a time**, alternating: Go concepts (goroutine, channel, context, interface), project decisions (why singleflight, why micro-dollars) and production scenarios (the provider went down, the client disconnected, two instances on Cloud Run).
3. Wait for the answer and assess it: what was right, what was missing, and a model answer of up to 5 lines citing the file in the repo.
4. At the end, give the 3 weakest points and which file to re-read for each.

Rules: don't make up how Globo works internally, only what is public. Any number quoted must be in the README or in `.claude/docs/log.md`.
