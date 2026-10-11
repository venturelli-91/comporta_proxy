package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/venturelli-91/comporta_proxy/internal/armazenamento"
)

// autenticar exchanges "Bearer ck_..." for the client. The lookup is by SHA-256 hash in the snapshot's
// map: whoever is guessing doesn't control the hash, so the lookup time leaks nothing useful about
// the key (which is why ConstantTimeCompare isn't needed here).
func (s *Servidor) autenticar(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		texto, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		sn := s.snap.Load()
		id, achou := sn.chaves[armazenamento.HashChave(texto)]
		cli, existe := sn.clientes[id]
		if !ok || !strings.HasPrefix(texto, "ck_") || !achou || !existe {
			erro(w, http.StatusUnauthorized, "chave_invalida", "missing or invalid API key")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxCliente, cli)))
	}
}

func clienteDe(ctx context.Context) armazenamento.Cliente {
	return ctx.Value(ctxCliente).(armazenamento.Cliente)
}

// permitir (allow) applies the client's token bucket. It returns how long to wait when over the limit.
// The limiter is recreated when the admin changes the RPM.
// ponytail: a removed client leaves its limiter in the map (one per client ever created; AUTOINCREMENT
// keeps a new client from inheriting it). Delete it in removerCliente if the number of clients explodes.
func (s *Servidor) permitir(cli armazenamento.Cliente) (bool, time.Duration) {
	if cli.RPM <= 0 {
		return true, 0
	}
	s.limMu.Lock()
	l, ok := s.limites[cli.ID]
	if !ok || l.Burst() != cli.RPM { // the burst is the RPM itself: if the admin changed it, recreate
		l = rate.NewLimiter(rate.Limit(float64(cli.RPM)/60), cli.RPM)
		s.limites[cli.ID] = l
	}
	s.limMu.Unlock()
	res := l.Reserve()
	if d := res.Delay(); d > 0 {
		res.Cancel()
		return false, d
	}
	return true, 0
}

// ocupar takes one of the client's concurrent slots; soltar gives it back.
// ponytail: the same limit for every client; it becomes a client column when someone needs more.
func (s *Servidor) ocupar(cliente int64) bool {
	s.limMu.Lock()
	defer s.limMu.Unlock()
	if s.emCurso[cliente] >= s.porCliente {
		return false
	}
	s.emCurso[cliente]++
	return true
}

func (s *Servidor) soltar(cliente int64) {
	s.limMu.Lock()
	defer s.limMu.Unlock()
	if s.emCurso[cliente]--; s.emCurso[cliente] <= 0 {
		delete(s.emCurso, cliente)
	}
}

func retryAfter(w http.ResponseWriter, d time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(d.Seconds()))))
}

// orcamentoEsgotado (budget exhausted) compares the month's spend (in memory) with the client's limit.
func (s *Servidor) orcamentoEsgotado(cli armazenamento.Cliente) bool {
	if cli.OrcamentoMicro <= 0 {
		return false
	}
	s.gastoMu.Lock()
	defer s.gastoMu.Unlock()
	s.virarMes()
	return s.gasto[cli.ID] >= cli.OrcamentoMicro
}

func (s *Servidor) gastar(cliente, micro int64) {
	if micro == 0 {
		return
	}
	s.gastoMu.Lock()
	defer s.gastoMu.Unlock()
	s.virarMes()
	s.gasto[cliente] += micro
}

// virarMes resets the spend when the (UTC) month changes. Call with gastoMu held.
func (s *Servidor) virarMes() {
	if mes := time.Now().UTC().Format("2006-01"); mes != s.gastoMes {
		s.gastoMes, s.gasto = mes, map[int64]int64{}
	}
}

const (
	cookieAdmin   = "comporta_admin"
	duracaoSessao = 12 * time.Hour
)

// Dashboard sessions: the cookie carries a random value and the server keeps only its SHA-256,
// with the expiry. Logout deletes the session, so a copied cookie stops working immediately.
// ponytail: in-memory sessions (lost on restart, one extra login); Redis when there are several instances.
func hashSessao(v string) string {
	h := sha256.Sum256([]byte(v))
	return hex.EncodeToString(h[:])
}

func (s *Servidor) novaSessao() string {
	v := rand.Text()
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	agora := time.Now()
	for k, expira := range s.sessoes { // clean up the expired ones while we're here
		if agora.After(expira) {
			delete(s.sessoes, k)
		}
	}
	s.sessoes[hashSessao(v)] = agora.Add(duracaoSessao)
	return v
}

func (s *Servidor) sessaoValida(v string) bool {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	expira, ok := s.sessoes[hashSessao(v)]
	return ok && time.Now().Before(expira)
}

func (s *Servidor) encerrarSessao(v string) {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	delete(s.sessoes, hashSessao(v))
}

func iguais(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

func (s *Servidor) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok := false
		if t, temBearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); temBearer {
			ok = iguais(t, s.d.AdminToken)
		} else if c, err := r.Cookie(cookieAdmin); err == nil {
			ok = s.sessaoValida(c.Value)
		}
		if !ok {
			erro(w, http.StatusUnauthorized, "nao_autorizado", "admin login required")
			return
		}
		h(w, r)
	}
}
