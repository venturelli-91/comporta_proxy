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

## Telas
1. **Visão geral:** cartões (requisições, custo, economia do cache, p95, taxa de erro, PII mascarada), gráfico de requisições por minuto, tabela por cliente e estado dos provedores.
2. **Clientes e chaves:** tabela com RPM, orçamento, cache e LGPD; formulário de criação; a chave nova aparece num aviso com botão de copiar, uma única vez.
3. **Shadow:** por alias com sombra, mostra principal × sombra em latência média, custo médio e tamanho médio da resposta.
4. **Playground:** chave `ck_`, alias, mensagem, stream ligado ou desligado; resposta ao vivo, TTFT e tokens.

## Estados
Toda tela tem carregando (esqueleto), vazio (o que fazer a seguir) e erro (mensagem do `erro.mensagem` + tentar de novo).
