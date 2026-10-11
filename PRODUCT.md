# Product

## Register

product

## Users
The people who own AI cost and risk inside a company (platform engineering, tech leads) and need to see at a
glance who is using the models, how much it costs, whether the provider is healthy and whether personal data
is leaving. Today, also the recruiter and the interviewer who open the public demo
(https://comporta.onrender.com) for the first time and have a few minutes to understand the product.

## Product Purpose
Comporta is an LLM gateway: it sits between applications and AI providers and controls per-client keys,
limits and budgets, caching and deduplication, resilience (retry, circuit breaker, fallback), a shadow mode
for migrating models, and masking of Brazilian personal data (LGPD). The dashboard is where all of this
becomes visible. Success: within 10 seconds the visitor understands that the provider received `[CPF_1]` and
the user got the CPF back; within 1 minute they understand cost, cache and provider health.

## Brand Personality
Reliable, precise, calm. The tone of a serious operations tool: numbers first, short and direct sentences in
English, no exclamation marks and no marketing. Color exists to convey state (ok, warning, error), not to
decorate.

## Anti-references
- SaaS template: generic gradient cards, giant marketing numbers, icon + title + text repeated in a grid.
- Decorative glassmorphism, gradient text, uppercase eyebrows above every section.
- A dark neon "gamer" dashboard just because technical tools "look cool" dark.

Positive references: Grafana and Datadog (dense observability, state front and center) and Vercel and Linear
(firm typography, careful dark mode, precise interaction).

## Design Principles
1. **Show, don't tell:** LGPD masking appears as a real before/after, not as text explaining it.
2. **Numbers are the content:** hierarchy comes from the data (large value, quiet label), not from decoration.
3. **Color is state:** teal for normal/action, amber for warning, red for error; everything else is neutral.
4. **Clickable looks clickable:** every interactive element has a visible pointer, hover and focus.
5. **Calm under load:** refresh every 5 s without layout shifts; loading shows skeletons.

## Accessibility & Inclusion
WCAG 2.2 AA: 4.5:1 contrast for text (3:1 for large text and components), full keyboard navigation with
visible focus, `prefers-reduced-motion` respected, state never conveyed by color alone (always with a label),
light, dark or system theme at the user's choice.
