# Arquitetura do front-end (painel)

## Stack
Vite + React + TypeScript, TanStack Query (estado de servidor), Recharts (gráficos), Tailwind (estilo).
Sem roteador externo: quatro abas com estado local e `location.hash`.
<!-- ponytail: react-router entra quando houver rotas aninhadas ou links profundos com parâmetros -->

## Estrutura
```
web/
  embed.go              //go:embed dist → o Go serve o painel no mesmo binário
  index.html
  src/
    main.tsx            QueryClient + App
    api.ts              fetch com credentials: "include"; erro {erro:{codigo,mensagem}} vira Error
    App.tsx             login (ADMIN_TOKEN → cookie) + abas
    telas/Visao.tsx     cartões + gráfico por minuto + tabela por cliente
    telas/Clientes.tsx  CRUD de clientes, gerar/revogar chave (mostrada uma vez)
    telas/Shadow.tsx    comparação principal × sombra
    telas/Playground.tsx fetch + ReadableStream do SSE, TTFT e custo
```

## Dados
| Tela | Endpoint | Atualização |
|---|---|---|
| Visão geral | `GET /admin/uso?de=&ate=` | a cada 5 s (`refetchInterval`) |
| Provedores | `GET /admin/provedores` | a cada 5 s |
| Clientes | `GET/POST/PUT/DELETE /admin/clientes`, `/admin/clientes/{id}/chaves` | invalida a query após cada mutação |
| Playground | `POST /v1/chat/completions` com a chave `ck_` colada | sob demanda |

## Segurança
- Login troca o `ADMIN_TOKEN` por um cookie `HttpOnly; SameSite=Strict`. Nada vai para o `localStorage`.
- A chave `ck_` do playground fica só na memória do componente.

## Build
`npm run build` gera `web/dist`. O Go embute com `//go:embed all:dist` e faz fallback para `index.html`.
Em desenvolvimento, `npm run dev` faz proxy de `/admin` e `/v1` para `:8080`.
