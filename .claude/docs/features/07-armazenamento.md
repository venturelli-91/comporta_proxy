# 07 — Armazenamento (SQLite)

**Pacote:** `internal/armazenamento` (`banco.go`, `uso.go`)

## O que faz
- Clientes (RPM, orçamento mensal, cache, LGPD, aliases liberados), com CRUD.
- Chaves `ck_...`: geradas com `crypto/rand.Text()` (128 bits), mostradas **uma vez** e guardadas só como SHA-256.
- `ChavesAtivas()` devolve hash → cliente, para o snapshot de autenticação em memória.
- `Gravador`: canal com buffer + goroutine que grava em lote (500 eventos ou 1 s). Canal cheio = descarta e conta.
- `Relatorio`: totais, p95, por cliente, por modelo, série por minuto e comparação shadow. `GastoDoMes` serve ao orçamento.

## Por que assim (decisões)
- **modernc.org/sqlite:** sem cgo, então o binário compila em qualquer lugar e a imagem distroless funciona.
- **WAL + busy_timeout:** o painel lê enquanto o gravador escreve.
- **SHA-256 sem sal:** a chave tem 128 bits aleatórios, sem dicionário para atacar. Bcrypt é para senha humana.
- **Gravação em lote, fora do caminho da requisição:** um INSERT por requisição somaria latência e disputaria o lock de escrita.
- **Shutdown drena o canal:** nenhum evento já aceito se perde.
- **p95 por posto mais próximo** com `ORDER BY ... LIMIT 1 OFFSET`, porque o SQLite não tem percentil.
- **Shadow fora dos totais e do orçamento:** a chamada sombra não foi respondida a ninguém e é decisão do admin, não do cliente.
- Ponytail: sem a interface `Repositorio` (uma implementação só). O relatório varre a tabela bruta; `uso_minuto` entra quando aparecer no profile.

## Como testar
`go test ./internal/armazenamento/ -v`: CRUD, chave só como hash, revogação, lote drenado no shutdown, canal cheio sem travar, relatório com p95, filtro por cliente, shadow e gasto do mês.

## Go vs TypeScript
- `database/sql` é a interface genérica, e o driver entra por `import _` (efeito colateral de registro). No Node, cada driver tem a sua própria API.
- `select { case ch <- e: ... default: ... }` é um envio não bloqueante, sem equivalente direto em Promise.
- `defer tx.Rollback()` depois do `Commit` não faz nada. É o padrão "rollback se não chegou ao commit".

## Exercício
Implemente a tabela `uso_minuto`, agregada no mesmo lote, e faça a `Serie` do relatório ler dela. Compare o tempo com 1 milhão de linhas.

## Pergunta de entrevista
"O que acontece com os registros de uso se o banco ficar lento, e por que você aceitou perder dados ali?"
