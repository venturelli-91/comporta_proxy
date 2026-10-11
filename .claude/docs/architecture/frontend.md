# Front-end architecture (dashboard)

## Stack
Vite + React + TypeScript, TanStack Query (server state), Recharts (charts), Tailwind (styling).
No external router: four tabs with local state and `location.hash`.
<!-- ponytail: react-router comes in when there are nested routes or deep links with parameters -->

## Structure
File names are still in Portuguese (phase 2 of the translation); UI text is in English.
```
web/
  embed.go              //go:embed dist → Go serves the dashboard from the same binary
  index.html
  src/
    main.tsx              QueryClient + App; a 401 on any screen goes back to login
    api.ts                fetch with credentials: "same-origin"; an {erro:{codigo,mensagem}} error becomes ErroApi
    ui.tsx                shared components (button, field, card, badge, skeleton, icons)
    App.tsx               login (ADMIN_TOKEN → cookie) + tabs
    telas/Visao.tsx       overview: cards + per-minute chart + per-client table
    telas/Clientes.tsx    clients CRUD, create/revoke key (shown once)
    telas/Shadow.tsx      primary × shadow comparison
    telas/Playground.tsx  fetch + ReadableStream of the SSE, what the provider received, TTFT and cost
    telas/Estado.tsx      loading skeleton and error with retry
```

## Data
| Screen | Endpoint | Refresh |
|---|---|---|
| Overview | `GET /admin/uso?de=&ate=` | every 5 s (`refetchInterval`) |
| Providers | `GET /admin/provedores` | every 5 s |
| Clients | `GET/POST/PUT/DELETE /admin/clientes`, `/admin/clientes/{id}/chaves` | invalidates the query after each mutation |
| Playground | `POST /v1/chat/completions` with the pasted `ck_` key | on demand |

## Security
- Login exchanges the `ADMIN_TOKEN` for an `HttpOnly; SameSite=Strict` cookie. Nothing goes into `localStorage`.
- The playground's `ck_` key lives only in the component's memory.

## Build
`npm run build` produces `web/dist`. Go embeds it with `//go:embed all:dist` and falls back to `index.html`.
In development, `npm run dev` proxies `/admin` and `/v1` to `:8080`.
