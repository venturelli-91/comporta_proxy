# Comporta — para quem não é técnico

## Em uma frase
A Comporta é uma "portaria" entre os sistemas da empresa e os serviços de inteligência artificial: controla
quem usa, quanto gasta e quais dados pessoais saem da empresa.

## Problema que resolve
Quando vários times passam a usar IA, quatro coisas acontecem:
1. **Custo descontrolado:** ninguém sabe quanto cada time gasta, e um sistema com defeito consome o orçamento do mês.
2. **Pagar duas vezes pela mesma resposta:** a mesma pergunta é enviada e cobrada várias vezes.
3. **Dependência de um fornecedor:** se o serviço de IA cai, tudo cai junto.
4. **Risco de LGPD:** CPF, e-mail e telefone de clientes acabam enviados a empresas de fora do Brasil.

## O que a Comporta faz
- Cada sistema recebe uma "credencial" própria, com limite de uso e de gasto mensal.
- Perguntas repetidas são respondidas da memória, sem pagar de novo.
- Se o fornecedor falha, ela tenta de novo ou usa um fornecedor reserva.
- CPF, CNPJ, e-mail, telefone e cartão são trocados por códigos antes de sair, e os dados originais voltam na resposta.
- Um painel mostra gasto, economia e quantos dados pessoais foram protegidos.

## Limites (dito com honestidade)
A proteção de dados reconhece formatos conhecidos. Nomes e endereços não são detectados. Ela reduz o risco, mas sozinha não garante conformidade com a LGPD.

## Status
Ver `.claude/docs/backlog.md`. Meta: MVP funcionando e medido até 13/10/2026.
