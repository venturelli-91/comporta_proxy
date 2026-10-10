# Design do painel

## Direção
Ferramenta de operação para quem cuida de custo e risco de IA: densa, legível e calma. A referência é
um painel de observabilidade, não uma landing page. Os números são o conteúdo; a cor só aparece
para dar estado (ok, alerta, erro).

## Tokens
| Token | Claro | Escuro | Uso |
|---|---|---|---|
| `--fundo` | `#f7f7f5` | `#111214` | página |
| `--superficie` | `#ffffff` | `#1a1b1e` | cartões, tabelas |
| `--texto` | `#1b1c1f` | `#e8e8ea` | texto principal |
| `--suave` | `#6b6e76` | `#9a9da5` | rótulos, eixos |
| `--borda` | `#e3e3e0` | `#2a2c30` | divisórias |
| `--acento` | `#0f766e` | `#2dd4bf` | ação primária, série principal |
| `--alerta` | `#b45309` | `#f59e0b` | breaker meio-aberto, orçamento > 80% |
| `--erro` | `#b91c1c` | `#f87171` | breaker aberto, 5xx |

Tipografia: system-ui para a interface; números em `font-variant-numeric: tabular-nums`; monoespaçada só para chaves e IDs.

Tema: segue o sistema por padrão; o seletor na navegação (claro, escuro, sistema) grava em `localStorage` e o `index.html` aplica `data-tema` antes da primeira pintura. Tokens extras: `--lateral` (navegação), `--realce` (hover), `--sobre-acento` (texto no botão teal). Componentes compartilhados em `web/src/ui.tsx` (botão, campo, cartão, selo, esqueleto, ícones SVG inline). Tudo que é clicável tem ponteiro, hover e foco visível.

## Telas
1. **Visão geral:** cartões (requisições, custo, economia do cache, p95, taxa de erro, PII mascarada), gráfico de requisições por minuto, tabela por cliente e estado dos provedores.
2. **Clientes e chaves:** tabela com RPM, orçamento, cache e LGPD; formulário de criação; a chave nova aparece num aviso com botão de copiar, uma única vez.
3. **Shadow:** por alias com sombra, mostra principal × sombra em latência média, custo médio e tamanho médio da resposta.
4. **Playground:** chave `ck_`, alias, mensagem, stream; lado a lado "O que o provedor recebeu" (cabeçalho `X-Comporta-Enviado`, marcadores realçados) e "O que voltou para você"; chips com TTFT, total, tokens e request id.

## Estados
Toda tela tem carregando (esqueleto), vazio (o que fazer a seguir) e erro (mensagem do `erro.mensagem` + tentar de novo).
