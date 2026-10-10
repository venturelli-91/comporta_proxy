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

## Status (09/10/2026)
O MVP está pronto e medido, quatro dias antes da meta (13/10):
- Aguenta **1.000 chamadas por segundo** acrescentando menos de **2 milésimos de segundo** a cada uma.
- Quando a mesma pergunta se repete, a resposta vem da memória: **99,9% das chamadas pagas evitadas** no teste.
- Com o fornecedor de IA **falhando metade das vezes**, **98,4%** das chamadas ainda terminaram bem.
- Em todos os testes, CPF e e-mail saíram trocados por códigos e voltaram corretos para o usuário.

Próximos passos (depois de 14/10): rodar várias cópias ao mesmo tempo na nuvem (Google Cloud) e alertas de orçamento.
