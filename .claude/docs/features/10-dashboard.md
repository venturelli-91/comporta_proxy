# 10 — React dashboard

**Folder:** `web/` (Vite + React 19 + TS + TanStack Query + Recharts + Tailwind 4), embedded in the binary through `web/embed.go`.

## What it does
- **Overview:** the gateway's "flow" (calls paid to the provider × answered by the cache × blocked), spend, savings, p95, error rate, protected personal data, per-minute chart, per-client table and breaker state. Refreshes every 5 s.
- **Clients and keys:** registration (RPM, budget, LGPD, cache, allowed models), create a key (shown once, with a copy button), revoke, remove.
- **Migration (shadow):** primary × candidate in average latency, cost and size.
- **Playground:** calls `/v1/chat/completions` with a `ck_` key, with streaming (`fetch` + `ReadableStream`), showing what the provider received next to the response, TTFT, total and tokens.

![Overview](../../../docs/img/painel.png)

## Why this way (decisions)
- **One standout element:** the flow bar translates the product ("comporta" is Portuguese for a floodgate controlling the flow). Everything else is tables and tabular text, no grid of cards.
- **A water palette** (teal over cool grey-green), color only for state; light, dark or system theme chosen in the navigation.
- **No react-router:** four tabs via `location.hash`. Ponytail: the router comes in with nested routes.
- **Login → HttpOnly cookie**, never a token in `localStorage`. The playground's `ck_` key lives only in the component's state. Only a 401 sends you to the login screen; other errors show a retry.
- **Money:** below 1 cent, it shows 2 significant digits (`$0.000017`), because per-call costs are fractions of a cent.

## Bug found in the visual smoke test
The screen got stuck on the login even with a valid cookie: in TanStack Query v5 a query can't resolve to `undefined`, and `/admin/sessao` returns 204. Now the `queryFn` returns `true`. Only the screenshot caught it: the build and the types passed.

## How to test
```bash
cd web && npm install && npm run dev    # :5173, proxying to Go on :8080
npm run build                           # produces web/dist; go build embeds it
```
Smoke on 10/09: headless Chrome via the DevTools Protocol (a screenshot script), screens at 1280 px and 390 px, all with real data from the fake provider.

## Go vs TypeScript
- `//go:embed all:dist` puts the build inside the binary: a single-file deploy, no Nginx.
- The fallback to `index.html` (`internal/httpapi/servidor.go`, `painel()`) is the equivalent of Vite's `historyApiFallback`.

## Exercise
Add a period filter (1 h, 24 h, 7 days) to the Overview using `/admin/uso`'s `?de=&ate=`, keeping the state in the hash.

## Interview question
"Why does the playground use `fetch` with `ReadableStream` instead of `EventSource` to read the SSE?"
