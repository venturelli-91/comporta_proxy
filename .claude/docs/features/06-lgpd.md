# 06 — LGPD: mascaramento e restauração

**Pacote:** `internal/lgpd` (`lgpd.go`)

## O que faz
- `Mascarar` troca CNPJ, CPF, cartão, e-mail e telefone por marcadores (`[CPF_1]`, `[EMAIL_2]`...) antes de o texto sair para o provedor.
- `Restaurar` devolve os valores reais na resposta. `NovoStream` faz o mesmo numa resposta que chega em pedaços.
- `Len` conta quantos valores foram protegidos (vai para o painel).

## Por que assim (decisões)
- **Dígito verificador (CPF/CNPJ) e Luhn (cartão):** regex pura marcaria qualquer número de 11 dígitos.
- **Ordem dos tipos:** CNPJ → CPF → cartão → e-mail → telefone. Um CPF sem pontuação também parece telefone. O validado ganha.
- **Pedaço de número maior não conta:** um CPF válido dentro de um protocolo de 13 dígitos fica como está.
- **Mesmo valor, mesmo marcador:** o modelo mantém a coerência ("o CPF [CPF_1]... confirme o [CPF_1]").
- **Marcador nunca colide com o texto do usuário:** se ele escreveu `[CPF_1]` literalmente, o CPF real vira `[CPF_2]`. É isso que torna `Restaurar(Mascarar(s)) == s` verdadeiro para qualquer `s`, e o fuzz test prova.
- **Stream:** segura só o trecho depois do último `[` enquanto ele ainda pode ser começo de marcador (menor que o maior marcador e sem `]`). O resto sai na hora, então o TTFT não piora.
- **Determinístico:** o mesmo texto sempre gera a mesma saída, o que preserva o cache do gateway e o cache de prefixo da DeepSeek.
- **O mapa vive só na requisição:** nunca vai para log, cache ou banco.
- Ponytail: `strings.Map` no lugar do laço manual que extraía os dígitos.

## Limitações (para o README)
Nomes, endereços e dados sem formato fixo não são detectados. Telefones fora do padrão brasileiro também não. Com várias mensagens na mesma requisição, um marcador literal escrito numa mensagem pode coincidir com um marcador real gerado em outra: o efeito máximo é o usuário receber o próprio dado de volta.

## Como testar
```
go test ./internal/lgpd/ -v
go test ./internal/lgpd/ -run x -fuzz FuzzMascararRestaurar -fuzztime 30s
go test ./internal/lgpd/ -run x -fuzz FuzzStream -fuzztime 30s
```
Em 09/10: 500 mil e 3,2 milhões de execuções sem falha.

## Go vs TypeScript
- O `regexp` do Go é RE2: sem lookbehind e sem backtracking catastrófico (tempo linear garantido). Por isso a checagem de "dígito ao lado" é feita à mão.
- Fuzz test é nativo (`go test -fuzz`). No TS seria o `fast-check`.
- `strings.Builder` evita a realocação de concatenar strings no laço, como um `array.join("")`.

## Exercício
Acrescente a detecção de CEP (`\d{5}-?\d{3}`), mas só quando vier depois da palavra "CEP". Escreva os falsos positivos na tabela antes.

## Pergunta de entrevista
"Como você garante que restaurar nunca altera um texto que o usuário digitou, e como provou isso?"
