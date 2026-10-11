# 06 — LGPD: masking and restoring

**Package:** `internal/lgpd` (`lgpd.go`)

LGPD is Brazil's data protection law. CPF and CNPJ are the Brazilian individual and company taxpayer IDs.

## What it does
- `Mascarar` (mask) replaces CNPJ, CPF, card, email and phone with placeholders (`[CPF_1]`, `[EMAIL_2]`...) before the text leaves for the provider. `MascararVarios` does it for all the messages of a request at once.
- `Restaurar` (restore) puts the real values back in the response. `NovoStream` does the same for a response that arrives in chunks.
- `Len` counts how many values were protected (it goes to the dashboard).

## Why this way (decisions)
- **Check digit (CPF/CNPJ) and Luhn (card):** a plain regex would flag any 11-digit number.
- **Type order:** CNPJ → CPF → card → email → phone. A CPF without punctuation also looks like a phone number. The validated one wins.
- **A piece of a bigger number doesn't count:** a valid CPF inside a 13-digit protocol number stays as is.
- **Same value, same placeholder:** the model stays coherent ("the CPF [CPF_1]... confirm [CPF_1]").
- **A placeholder never collides with the user's text:** if they literally wrote `[CPF_1]`, the real CPF becomes `[CPF_2]`. Placeholders avoid the text of every message in the request. That's what makes `Restaurar(Mascarar(s)) == s` true for any `s`, and the fuzz test proves it.
- **Stream:** holds back only the part after the last `[` while it can still be the start of a placeholder (shorter than the longest placeholder and without `]`). Everything else goes out right away, so TTFT doesn't get worse.
- **Deterministic:** the same text always produces the same output, which preserves the gateway cache and DeepSeek's prefix cache.
- **The map lives only in the request:** never in logs, cache or database.
- Ponytail: `strings.Map` instead of the manual loop that extracted the digits.

## Limitations (for the README)
Names, addresses and data without a fixed format aren't detected. Neither are phone numbers outside the Brazilian pattern. Only text `messages[].content` is masked: multimodal content and tool-call arguments pass through unmasked.

## How to test
```
go test ./internal/lgpd/ -v
go test ./internal/lgpd/ -run x -fuzz FuzzMascararRestaurar -fuzztime 30s
go test ./internal/lgpd/ -run x -fuzz FuzzStream -fuzztime 30s
```
On 10/09: 500 thousand and 3.2 million executions without failure.

## Go vs TypeScript
- Go's `regexp` is RE2: no lookbehind and no catastrophic backtracking (guaranteed linear time). That's why the "digit next to it" check is done by hand.
- Fuzz testing is built in (`go test -fuzz`). In TS it would be `fast-check`.
- `strings.Builder` avoids the reallocation of concatenating strings in a loop, like an `array.join("")`.

## Exercise
Add detection of CEP (Brazilian postal code, `\d{5}-?\d{3}`), but only when it follows the word "CEP". Write the false positives in the table first.

## Interview question
"How do you guarantee that restoring never changes text the user typed, and how did you prove it?"
