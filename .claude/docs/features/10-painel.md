# 10 — Painel React

**Pasta:** `web/` (Vite + React 19 + TS + TanStack Query + Recharts + Tailwind 4), embutido no binário via `web/embed.go`.

## O que faz
- **Visão geral:** a "vazão" da comporta (chamadas pagas ao provedor × respondidas pelo cache × barradas), gasto, economia, p95, taxa de erro, dados pessoais protegidos, gráfico por minuto, tabela por cliente e estado dos breakers. Atualiza a cada 5 s.
- **Clientes e chaves:** cadastro (RPM, orçamento, LGPD, cache, modelos liberados), gerar chave (mostrada uma vez, com botão de copiar), revogar, remover.
- **Migração (shadow):** principal × candidato em latência, custo e tamanho médios.
- **Playground:** chama `/v1/chat/completions` com uma chave `ck_`, com stream (`fetch` + `ReadableStream`), mostrando o TTFT, o total e os tokens.

![Visão geral](../../../docs/img/painel.png)

## Por que assim (decisões)
- **Elemento marcante, um só:** a barra de vazão traduz o produto (uma comporta controlando o fluxo). O resto é tabela e texto tabular, sem grade de cartões.
- **Paleta de água** (teal sobre cinza-esverdeado frio), cor só para estado; dark mode por `prefers-color-scheme`.
- **Sem react-router:** quatro abas por `location.hash`. Ponytail: o roteador entra com rotas aninhadas.
- **Login → cookie HttpOnly**, nunca token no `localStorage`. A chave `ck_` do playground fica só no estado do componente.
- **Dinheiro:** abaixo de 1 centavo, mostra 2 dígitos significativos (`$0.000017`), porque custos por chamada são frações de centavo.

## Bug encontrado no smoke visual
A tela ficava presa no login mesmo com o cookie válido: no TanStack Query v5, uma query não pode resolver `undefined`, e o `/admin/sessao` responde 204. Agora o `queryFn` devolve `true`. Só o screenshot pegou isso: o build e os tipos passavam.

## Como testar
```bash
cd web && npm install && npm run dev    # :5173, proxy para o Go em :8080
npm run build                           # gera web/dist; o go build embute
```
Smoke de 09/10: Chrome headless via DevTools Protocol (script de screenshot), telas em 1280 px e 390 px, todas com dados reais do provedor falso.

## Go vs TypeScript
- `//go:embed all:dist` põe o build dentro do binário: deploy de um arquivo só, sem Nginx.
- O fallback para `index.html` (`internal/httpapi/servidor.go`, `painel()`) é o equivalente ao `historyApiFallback` do Vite.

## Exercício
Acrescente um filtro de período (1 h, 24 h, 7 dias) na Visão geral usando `?de=&ate=` do `/admin/uso`, mantendo o estado no hash.

## Pergunta de entrevista
"Por que o playground usa `fetch` com `ReadableStream` em vez de `EventSource` para ler o SSE?"
