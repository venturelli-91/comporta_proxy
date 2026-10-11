# Dashboard design

## Direction
An operations tool for whoever owns AI cost and risk: dense, legible and calm. The reference is an
observability dashboard, not a landing page. The numbers are the content; color only appears to convey
state (ok, warning, error).

## Tokens
CSS token names are still in Portuguese (phase 2); the English meaning is in the Use column.

| Token | Light | Dark | Use |
|---|---|---|---|
| `--fundo` | `#f7f7f5` | `#111214` | page background |
| `--superficie` | `#ffffff` | `#1a1b1e` | surface: cards, tables |
| `--texto` | `#1b1c1f` | `#e8e8ea` | main text |
| `--suave` | `#6b6e76` | `#9a9da5` | muted: labels, axes |
| `--borda` | `#e3e3e0` | `#2a2c30` | borders and dividers |
| `--acento` | `#0f766e` | `#2dd4bf` | accent: primary action, main series |
| `--alerta` | `#b45309` | `#f59e0b` | warning: half-open breaker, budget > 80% |
| `--erro` | `#b91c1c` | `#f87171` | error: open breaker, 5xx |

Typography: system-ui for the interface; numbers in `font-variant-numeric: tabular-nums`; monospace only for keys and IDs.

Theme: follows the system by default; the selector in the navigation (light, dark, system) is saved in `localStorage` and `index.html` applies `data-tema` before the first paint. Extra tokens: `--lateral` (navigation), `--realce` (hover highlight), `--sobre-acento` (text on the teal button). Shared components in `web/src/ui.tsx` (button, field, card, badge, skeleton, inline SVG icons). Everything clickable has a pointer, hover and visible focus.

## Screens
1. **Overview:** cards (requests, cost, cache savings, p95, error rate, masked PII), requests-per-minute chart, per-client table and provider state.
2. **Clients and keys:** table with RPM, budget, cache and LGPD; creation form; the new key appears in a notice with a copy button, exactly once.
3. **Migration (shadow):** for each alias with a shadow, primary × shadow in average latency, average cost and average response size.
4. **Playground:** `ck_` key, alias, message, stream; side by side "What the provider received" (`X-Comporta-Enviado` header, placeholders highlighted) and "What came back to you"; chips with TTFT, total, tokens and request id.

## States
Every screen has loading (skeleton), empty (what to do next) and error (the `erro.mensagem` message + try again).
