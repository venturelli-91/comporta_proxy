# Pendências pós-MVP (segurança do login + limpezas): plano de implementação

> **Para agentes:** SUB-SKILL OBRIGATÓRIA: superpowers:executing-plans (recomendado aqui) ou superpowers:subagent-driven-development. Passos com checkbox (`- [ ]`).
> Ao aprovar, copiar este arquivo para `docs/superpowers/plans/2026-10-10-pendencias-login.md` (o plan mode não deixa gravar lá agora).

**Goal:** fechar os 3 alertas de segurança do login, corrigir o M-5 e aplicar os cortes do ponytail-audit. Tudo num branch novo com PR para `main`, a tempo da entrevista de 14/10.

**Architecture:** são mudanças pontuais em `internal/httpapi` (login, IP, limitador) e `internal/armazenamento` (revogar chave). Não há pacote novo. A única config nova é `Deps.ConfiarProxy`, ligada em `main.go` por `K_SERVICE` (Cloud Run) ou `TRUST_PROXY=1`.

**Tech Stack:** Go 1.26 com stdlib e `golang.org/x/time/rate`. SQLite via `modernc.org/sqlite`.

**Spec:** topo de `.claude/docs/backlog.md` ("Próxima sessão — começar por aqui (P0)"), item M-5 e o relatório de ponytail-audit/debt desta sessão.

## Context
O PR #1 (MVP) já foi **mergeado**. `origin/main` só não tem o commit de docs `7ecc384`. Ficaram 3 falhas no limite de login (`admin.go`):
- **Corrida:** o código checa `Tokens()` e só consome depois.
- **`X-Forwarded-For` forjável:** sem proxy na frente, cada tentativa pode chegar com um IP novo.
- **Reset forçável do mapa de IPs.**

Também faltam:
- **M-5:** revogar uma chave ignora o cliente do path.
- Cortes de stdlib (~20 linhas).
- Backlog desatualizado.

## Global Constraints
- `gofmt`, `go vet` limpo e `go test -race ./...`. No Windows o `-race` só roda via Docker (Task 7).
- Commits em português, no formato Conventional Commits `tipo(escopo): descrição`, um por task, com a linha `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`.
- Comentários em português explicando o porquê. Atalho deliberado leva `// ponytail: <limite>, <gatilho>`.
- Nenhuma dependência nova.
- Modo aprendizado: na **Task 1 quem implementa é o Aurélio**. Eu escrevo o teste, ele escreve o código, eu reviso.

## Review Focus
1. **Admin legítimo atrás do Cloud Run:** com `ConfiarProxy` ligado, o IP vem da última entrada do XFF. O admin em outro IP continua logando mesmo com o atacante bloqueado. Teste na Task 1.
2. **Login certo agora consome token:** 5 logins certos seguidos do mesmo IP → o 6º leva 429. É aceitável, porque o limite é por IP e o atacante não afeta o admin. Precisa ficar documentado no teste da Task 1.
3. **XFF com espaços ou entrada única** (`"203.0.113.9"`, `"a, b"`): devolve a última, sem espaço. Teste na Task 2.
4. **Mapa acima de 10 mil com todos os limitadores gastos:** a poda não apaga nada, o mapa cresce e cada login varre O(n). Fica como limite conhecido, com `ponytail:` na Task 3.
5. **Revogar a chave de outro cliente pelo path** → 404, e a chave continua ativa. Teste na Task 4.

---

### Task 0: Branch
- [ ] `git switch -c fix/login-seguranca` a partir do `feat/mvp` atual (carrega o `7ecc384`).
- [ ] Copiar este plano para `docs/superpowers/plans/2026-10-10-pendencias-login.md` e fazer o commit `docs(plano): pendências de segurança do login`.

### Task 1: Consumir o limite na entrada (corrida) — **o Aurélio implementa**

**Files:** Modify `internal/httpapi/admin.go:35-48`; Test `internal/httpapi/chat_test.go` (perto da linha 627).

**Interfaces:** sem mudança de assinatura.

- [ ] **Step 1: teste paralelo que falha.** Acrescentar `sync/atomic` aos imports se faltar.
```go
// 20 tentativas erradas ao mesmo tempo do mesmo IP: no máximo 5 podem chegar à comparação.
// Com checar-e-depois-consumir, todas passavam pela checagem antes de alguma consumir.
func TestLoginParaleloRespeitaLimite(t *testing.T) {
	c := novoCenario(t)
	var wg sync.WaitGroup
	var comparadas atomic.Int32
	for range 20 {
		wg.Go(func() {
			req, _ := http.NewRequest("POST", c.url+"/admin/login", strings.NewReader(`{"token":"chute"}`))
			req.Header.Set("X-Forwarded-For", "203.0.113.9")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err) // t.Fatal não pode ser chamado fora da goroutine do teste
				return
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized {
				comparadas.Add(1)
			}
		})
	}
	wg.Wait()
	if n := comparadas.Load(); n > 5 {
		t.Fatalf("%d tentativas chegaram à comparação; o limite é 5", n)
	}
}
```
- [ ] **Step 2: ajustar `TestLoginLimitaTentativasSemTrancarOAdmin`.** Apagar o laço final "login certo não pode consumir o limite" e o comentário "só conta tentativa errada". No lugar, o comentário passa a dizer: "o limite é por IP; o login certo também consome, mas o atacante em outro IP não tranca o admin".
- [ ] **Step 3:** `go test ./internal/httpapi -run 'TestLogin' -count=5`. Esperado: o paralelo FALHA (passa de 5).
- [ ] **Step 4: implementação (Aurélio).** Código esperado na revisão:
```go
func (s *Servidor) login(w http.ResponseWriter, r *http.Request) {
	// Consome na entrada: checar e consumir num passo só fecha a corrida entre tentativas paralelas.
	// O login certo também gasta um token; como o limite é por IP, o atacante não tranca o admin.
	if !s.limiteLogin(s.ipDe(r)).Allow() {
		retryAfter(w, time.Minute)
		erro(w, http.StatusTooManyRequests, "muitas_tentativas", "muitas tentativas de login, aguarde um minuto")
		return
	}
	var v struct{ Token string }
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&v)
	if s.d.AdminToken == "" || !iguais(v.Token, s.d.AdminToken) {
		erro(w, http.StatusUnauthorized, "nao_autorizado", "token de admin inválido")
		return
	}
	// ... resto igual
```
(`s.ipDe` só nasce na Task 2. Nesta task fica `ipDe(r)`.)
- [ ] **Step 5:** `go test ./internal/httpapi -count=5` deve PASSAR.
- [ ] **Step 6:** commit `fix(httpapi): limite de login consumido na entrada, sem corrida`.

### Task 2: XFF só com proxy confiável

**Files:** Modify `internal/httpapi/servidor.go` (`Deps`), `internal/httpapi/admin.go:73-87` (`ipDe` vira método), `internal/httpapi/chat_test.go:119` (`novoCenario`), `cmd/comporta/main.go` (montagem de `Deps`). Test: `internal/httpapi/chat_test.go`.

**Interfaces:**
- Produz `Deps.ConfiarProxy bool`.
- Produz `func (s *Servidor) ipDe(r *http.Request) string`.

- [ ] **Step 1: teste table-driven (falha porque não compila).**
```go
// Sem proxy confiável na frente, o XFF é do cliente: trocá-lo não pode render um balde novo.
func TestIPDe(t *testing.T) {
	casos := []struct {
		nome    string
		confiar bool
		xff     string
		quer    string
	}{
		{"sem proxy ignora xff", false, "1.2.3.4, 203.0.113.9", "192.0.2.1"}, // RemoteAddr do httptest
		{"cloud run usa a última", true, "1.2.3.4,  203.0.113.9", "203.0.113.9"},
		{"cloud run entrada única", true, "203.0.113.9", "203.0.113.9"},
		{"cloud run sem xff", true, "", "192.0.2.1"},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			s := &Servidor{d: Deps{ConfiarProxy: tc.confiar}}
			r := httptest.NewRequest("POST", "/admin/login", nil)
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := s.ipDe(r); got != tc.quer {
				t.Fatalf("ipDe = %q, queria %q", got, tc.quer)
			}
		})
	}
}
```
- [ ] **Step 2:** `go test ./internal/httpapi -run TestIPDe` → FALHA (não compila).
- [ ] **Step 3: implementação.**
  - Em `Deps`:
    ```go
    // ConfiarProxy: há um proxy na frente (Cloud Run) que acrescenta o IP real ao X-Forwarded-For.
    // Sem ele, o cabeçalho é do cliente e não serve para identificar ninguém.
    ConfiarProxy bool
    ```
  - Em `admin.go`, `ipDe` vira:
    ```go
    // ipDe usa a última entrada do X-Forwarded-For, a que o proxy confiável acrescenta (o cliente
    // só forja as anteriores), e só quando há esse proxy. Sem ele, vale o endereço da conexão.
    func (s *Servidor) ipDe(r *http.Request) string {
    	if xff := r.Header.Get("X-Forwarded-For"); s.d.ConfiarProxy && xff != "" {
    		return strings.TrimSpace(xff[strings.LastIndex(xff, ",")+1:])
    	}
    	host, _, err := net.SplitHostPort(r.RemoteAddr)
    	if err != nil {
    		return r.RemoteAddr
    	}
    	return host
    }
    ```
    A chamada em `login` passa a ser `s.ipDe(r)`.
  - Em `novoCenario`, adicionar `ConfiarProxy: true` a `Deps{...}`, com o comentário `// os testes simulam o Cloud Run: o XFF separa os IPs`.
  - Em `main.go`, adicionar ao `httpapi.Deps{...}`:
    ```go
    ConfiarProxy: os.Getenv("K_SERVICE") != "" || os.Getenv("TRUST_PROXY") == "1",
    ```
- [ ] **Step 4:** `go test ./internal/httpapi` → PASSA (inclui os testes de login da Task 1).
- [ ] **Step 5:** commit `fix(httpapi): X-Forwarded-For só vale atrás de proxy confiável`.

### Task 3: Podar só os limitadores cheios

**Files:** Modify `internal/httpapi/admin.go:56-71`. Test: `internal/httpapi/admin_test.go` (arquivo novo e pequeno, `package httpapi`, para testar o mapa interno).

- [ ] **Step 1: teste que falha.**
```go
package httpapi

import (
	"strconv"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// Passar de 10 mil IPs não pode devolver tentativas a quem já gastou as suas.
func TestLimiteLoginPodaSoOsCheios(t *testing.T) {
	s := &Servidor{tentativas: map[string]*rate.Limiter{}}
	for i := range 10_000 {
		s.tentativas[strconv.Itoa(i)] = rate.NewLimiter(rate.Every(12*time.Second), 5)
	}
	gasto := rate.NewLimiter(rate.Every(12*time.Second), 5)
	for range 5 {
		gasto.Allow()
	}
	s.tentativas["203.0.113.9"] = gasto

	s.limiteLogin("198.51.100.7") // 10.001 entradas: dispara a poda

	if s.tentativas["203.0.113.9"] != gasto {
		t.Fatal("o limitador gasto foi apagado: o atacante ganhou tentativas novas")
	}
	if n := len(s.tentativas); n != 2 {
		t.Fatalf("sobraram %d limitadores; queria só o gasto e o novo", n)
	}
}
```
- [ ] **Step 2:** `go test ./internal/httpapi -run TestLimiteLoginPodaSoOsCheios` → FALHA (o gasto é apagado).
- [ ] **Step 3: implementação.** Trocar o reset por:
```go
// limiteLogin: 5 tentativas por IP, uma nova a cada 12 s. Por IP para que um atacante não
// tranque o admin de verdade do lado de fora.
// ponytail: poda varre o mapa inteiro e só tira os cheios; com 10 mil IPs gastos ao mesmo tempo
// o mapa cresce e cada login fica O(n). LRU com teto se isso virar alvo.
func (s *Servidor) limiteLogin(ip string) *rate.Limiter {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	if len(s.tentativas) > 10_000 {
		for k, l := range s.tentativas {
			if l.Tokens() >= float64(l.Burst()) { // cheio = igual a um novo, não guarda nada
				delete(s.tentativas, k)
			}
		}
	}
	// ... resto igual
```
- [ ] **Step 4:** `go test ./internal/httpapi` → PASSA.
- [ ] **Step 5:** commit `fix(httpapi): mapa de tentativas poda só limitadores cheios`.

### Task 4: M-5, revogar respeita o cliente do path

**Files:** Modify `internal/armazenamento/banco.go:168-171`, `internal/httpapi/admin.go:202-206`. Test: `internal/armazenamento/banco_test.go:72`.

**Interfaces:** `func (b *Banco) RevogarChave(ctx context.Context, clienteID, id int64) error`. Devolve `ErrNaoEncontrado` se a chave não for desse cliente.

- [ ] **Step 1: teste.** Em `TestRevogarChave`, trocar a chamada por `b.RevogarChave(ctx, c.ID, chave.ID)` e acrescentar no início:
```go
	outro, _ := b.CriarCliente(ctx, Cliente{Nome: "outro"})
	if err := b.RevogarChave(ctx, outro.ID, chave.ID); !errors.Is(err, ErrNaoEncontrado) {
		t.Fatalf("revogar pela rota de outro cliente: err = %v, queria ErrNaoEncontrado", err)
	}
	if ativas, _ := b.ChavesAtivas(ctx); len(ativas) != 1 {
		t.Fatal("a chave foi revogada pela rota de outro cliente")
	}
```
(Adicionar `errors` ao import se faltar.)
- [ ] **Step 2:** `go test ./internal/armazenamento -run TestRevogarChave` → FALHA (não compila).
- [ ] **Step 3: implementação.**
```go
func (b *Banco) RevogarChave(ctx context.Context, clienteID, id int64) error {
	res, err := b.db.ExecContext(ctx, `UPDATE chaves SET revogada_em = ? WHERE id = ? AND cliente_id = ? AND revogada_em IS NULL`,
		time.Now().Unix(), id, clienteID)
	return linhaAfetada(res, err, "revogando chave")
}
```
No handler:
```go
func (s *Servidor) revogarChave(w http.ResponseWriter, r *http.Request) {
	cliente, ok := idDe(w, r, "id")
	if !ok {
		return
	}
	if id, ok := idDe(w, r, "chave"); ok {
		s.mutou(w, r, s.d.Banco.RevogarChave(r.Context(), cliente, id), http.StatusNoContent, nil)
	}
}
```
- [ ] **Step 4:** `go test ./...` → PASSA (o teste HTTP de revogar em `chat_test.go:~420` usa o path certo).
- [ ] **Step 5:** commit `fix(armazenamento): revogar chave exige o cliente dono`.

### Task 5: Cortes do ponytail-audit

**Files:**
- `internal/httpapi/servidor.go` (`padrao`, `limitador`, `erro`)
- `internal/httpapi/auth.go:43-56` (`permitir`)
- `cmd/comporta/main.go:34-39` (`env`)
- `internal/gateway/roteador.go:195`

São refactors sem mudança de comportamento. A cobertura vem dos testes que já existem.

- [ ] `padrao(...)` → `cmp.Or(t.PrimeiroToken(), 30*time.Second)` (idem `EntreChunks` e `Total`). Apagar a função `padrao`. Diferença aceita: uma duração negativa na config deixaria de cair no padrão; a config é em ms e não gera negativos.
- [ ] `env(n, p)` em `main.go` → `cmp.Or(os.Getenv(n), p)` nas 4 chamadas. Apagar a função `env`.
- [ ] Apagar `type limitador`. O campo vira `limites map[int64]*rate.Limiter` e `permitir` fica:
```go
	l, ok := s.limites[cli.ID]
	if !ok || l.Burst() != cli.RPM { // o burst é o próprio RPM: mudou no admin, recria
		l = rate.NewLimiter(rate.Limit(float64(cli.RPM)/60), cli.RPM)
		s.limites[cli.ID] = l
	}
	s.limMu.Unlock()
	res := l.Reserve()
```
  A inicialização em `Novo` muda para `limites: map[int64]*rate.Limiter{}`.
- [ ] `erro(...)` vira uma linha: `responderJSON(w, status, map[string]any{"erro": map[string]string{"codigo": codigo, "mensagem": mensagem}})`.
- [ ] `roteador.go`: `slices.SortFunc(out, func(a, b EstadoDestino) int { return cmp.Compare(a.Destino, b.Destino) })`. Trocar o import `sort` por `cmp` e `slices`.
- [ ] `gofmt -l . && go vet ./... && go test ./...` → limpo e PASSA.
- [ ] Commit `refactor: cortes de stdlib apontados pela auditoria ponytail`.

### Task 6: Documentação
**Files:** `.claude/docs/backlog.md`, `.claude/docs/log.md`, `CLAUDE.md`, `docs/ARQUITETURA.md`, `README.md`.
- [ ] Backlog:
  - Marcar os 3 itens P0 e o M-5 como feitos.
  - Remover "Limite de concorrência por cliente" do P1 (já foi feito em `3bc3677`).
  - Acrescentar à tabela de dívida as linhas `admin.go` (poda O(n)), `auth.go` (limite único 32/chave), `auth.go` (sessões em memória) e `chat.go` (uso estimado sem marca).
  - Trocar o "fazer push / PR #1" pelo PR novo.
- [ ] `log.md`: uma linha por task (data 10/10).
- [ ] `CLAUDE.md`:
  - Em **Decisões**: "**XFF só atrás de proxy confiável** (`K_SERVICE`/`TRUST_PROXY=1`): sem proxy o cabeçalho é do cliente e daria um balde novo a cada tentativa."
  - Em **Variáveis**: acrescentar `TRUST_PROXY`.
- [ ] `ARQUITETURA.md` (seção admin) e `README.md` (variáveis): uma linha sobre `TRUST_PROXY` e o login limitado na entrada.
- [ ] Commit `docs: pendências de segurança do login resolvidas`.

### Task 7: Verificação com `-race` + PR
- [ ] `docker run --rm -v "$PWD":/src -w /src golang:1.26 go test -race -count=2 ./...`. A imagem Debian já tem gcc, então o cgo funciona. Esperado: todos os pacotes `ok`.
- [ ] `go vet ./...` local.
- [ ] Smoke manual com `go run ./cmd/comporta` e `curl`:
  - 6 POST `/admin/login` com token errado → os 5 primeiros dão 401 e o 6º dá 429.
  - Trocando o `X-Forwarded-For` continua 429, porque `TRUST_PROXY` não está definido.
- [ ] Agente `code-reviewer` no diff do branch.
- [ ] `git push -u origin fix/login-seguranca`.
- [ ] `gh pr create --base main` com título `fix: segurança do limite de login e revogação de chave`. Corpo: os 3 alertas, o M-5, os cortes e o resultado do `-race`, terminando com `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
- [ ] **Não fazer merge.** Fica para o Aurélio.

## Verificação end-to-end
`gofmt -l .` vazio · `go vet ./...` · `-race` no Docker verde (Task 7) · smoke do curl acima · CI/PR aberto contra `main`.

## Pergunta de entrevista (fim)
"Por que confiar na **última** entrada do `X-Forwarded-For` e não na primeira? E o que muda se houver dois proxies na frente (load balancer + Cloud Run)?"
