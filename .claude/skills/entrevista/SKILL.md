---
name: entrevista
description: Prepara o Aurélio para a entrevista de 14/10 (proxy em Go na GCP) a partir das features já construídas. Use quando ele pedir "me treina", "simulado", "perguntas de entrevista" ou "revisar o pitch".
---

# Treino de entrevista da Comporta

1. Leia `.claude/docs/features/*.md` (seções "Pergunta de entrevista" e "Decisões") e o pitch no `CLAUDE.md`.
2. Faça **uma pergunta por vez**, alternando: conceito de Go (goroutine, channel, context, interface), decisão de projeto (por que singleflight, por que micro-dólares) e cenário de produção (o provedor caiu, o cliente desconectou, duas instâncias no Cloud Run).
3. Espere a resposta e avalie: o que estava certo, o que faltou, e uma resposta modelo de até 5 linhas que cite o arquivo do repo.
4. No fim, dê os 3 pontos mais fracos e qual arquivo reler para cada um.

Regras: não invente como a Globo funciona por dentro, só o que é público. Números citados têm que estar no README ou em `.claude/docs/log.md`.
