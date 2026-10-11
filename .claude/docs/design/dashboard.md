# Dashboard design

## Direction
**Engineering instrument.** An operations tool for whoever owns AI cost and risk: dense, legible and calm,
read in a lit room or on a shared screen during a call. Light by default. Neutrals carry no hue, ink is
near-black, sections are separated by hairline rules instead of shadowed cards, and one deep teal is the
only accent. Color only conveys state (ok, warning, error).

The signature is the **floodgate** on the Overview: the calls that came in branch into paid, cache and
blocked, drawn as a 1px channel with proportional bars. The Playground's ink seals (`[CPF_1]` on near-black)
are the second recognizable element.

Why: the previous version (system-ui, rounded cards with shadows, grey-green background) read as a generic
template. The floodgate was the only original idea and it was a 12px bar.

## Tokens
CSS token names are still in Portuguese (phase 2); the English meaning is in the Use column. Values are
OKLCH in `web/src/index.css`.

| Token | Light | Dark | Use |
|---|---|---|---|
| `--fundo` | `oklch(0.985 0 0)` | `oklch(0.17 0.003 180)` | page background |
| `--lateral` | `oklch(0.955 0.002 180)` | `oklch(0.145 0.003 180)` | second layer: navigation, provider side, form panel |
| `--superficie` | `oklch(1 0 0)` | `oklch(0.2 0.004 180)` | containers that need one: tables, panels |
| `--realce` | `oklch(0.965 0.002 180)` | `oklch(0.23 0.004 180)` | hover highlight |
| `--texto` | `oklch(0.2 0.005 180)` | `oklch(0.94 0.003 180)` | ink; also the seal background |
| `--suave` | `oklch(0.47 0.006 180)` | `oklch(0.72 0.006 180)` | muted: labels, axes |
| `--borda` | `oklch(0.9 0.003 180)` | `oklch(0.3 0.004 180)` | rules and borders |
| `--acento` | `oklch(0.47 0.1 185)` | `oklch(0.78 0.12 180)` | primary action, normal state, main series |
| `--acento-claro` | `oklch(0.8 0.07 185)` | `oklch(0.45 0.07 185)` | cache branch of the floodgate |
| `--alerta` | `oklch(0.52 0.13 60)` | `oklch(0.8 0.15 75)` | warning: half-open breaker, errors > 0 |
| `--erro` | `oklch(0.53 0.19 27)` | `oklch(0.72 0.17 25)` | error: open breaker, error rate ≥ 5% |

`--sombra` is `none`: depth comes from rules and the second neutral layer. Corners are 2/3/4 px through
the Tailwind `--radius-sm/md/lg` tokens, so every `rounded-*` class follows without edits.

Typography: Schibsted Grotesk (variable) for all UI, JetBrains Mono only for identifiers (keys, model ids,
placeholders, request ids). Both self-hosted via `@fontsource-variable/*` and embedded in the binary.
Tabular figures only on `td`, `th`, `dd` and `data`: in this face `tnum` also widens `:` and `;` in prose.

Theme: follows the system by default; the selector in the navigation (light, dark, system) is saved in
`localStorage` and `index.html` applies `data-tema` before the first paint. Shared components in
`web/src/ui.tsx` (button, field, card, badge, skeleton, inline SVG icons, the floodgate mark). Everything
clickable has a pointer, hover and visible focus.

## Screens
1. **Overview:** the floodgate (came in → paid / cache / blocked), one ruled KPI strip (spend, cache
   savings, p95, error rate, masked PII), requests-per-minute chart, per-client table and provider state.
2. **Clients and keys:** ruled list with RPM, budget, cache and LGPD; creation form on the second layer; the
   new key appears in a notice with a copy button, exactly once.
3. **Migration (shadow):** for each alias with a shadow, primary × candidate in average latency, cost and
   response size; the difference carries ↑/↓, sign and color.
4. **Playground:** `ck_` key, alias, message, stream; side by side "Comporta → provider" (what the provider
   received, placeholders as ink seals, on the second layer because it's outside the company) and
   "Comporta → your app" (what came back); a ruled line with TTFT, total, tokens and request id.

## States
Every screen has loading (skeleton), empty (what to do next) and error (the `erro.mensagem` message + try again).
