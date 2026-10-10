package httpapi

import (
	"context"
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

// autenticar troca "Bearer ck_..." pelo cliente. A busca é pelo hash SHA-256 no mapa do
// snapshot: quem tenta adivinhar não controla o hash, então o tempo da busca não vaza
// nada útil sobre a chave (por isso aqui não precisa de ConstantTimeCompare).
func (s *Servidor) autenticar(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		texto, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		sn := s.snap.Load()
		id, achou := sn.chaves[armazenamento.HashChave(texto)]
		cli, existe := sn.clientes[id]
		if !ok || !strings.HasPrefix(texto, "ck_") || !achou || !existe {
			erro(w, http.StatusUnauthorized, "chave_invalida", "chave de API ausente ou inválida")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxCliente, cli)))
	}
}

func clienteDe(ctx context.Context) armazenamento.Cliente {
	return ctx.Value(ctxCliente).(armazenamento.Cliente)
}

// permitir aplica o token bucket do cliente. Devolve quanto esperar quando passou do limite.
// O limitador é recriado quando o admin muda o RPM.
func (s *Servidor) permitir(cli armazenamento.Cliente) (bool, time.Duration) {
	if cli.RPM <= 0 {
		return true, 0
	}
	s.limMu.Lock()
	l, ok := s.limites[cli.ID]
	if !ok || l.rpm != cli.RPM {
		l = limitador{cli.RPM, rate.NewLimiter(rate.Limit(float64(cli.RPM)/60), cli.RPM)}
		s.limites[cli.ID] = l
	}
	s.limMu.Unlock()
	res := l.l.Reserve()
	if d := res.Delay(); d > 0 {
		res.Cancel()
		return false, d
	}
	return true, 0
}

func retryAfter(w http.ResponseWriter, d time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(d.Seconds()))))
}

// orcamentoEsgotado compara o gasto do mês (em memória) com o limite do cliente.
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

// virarMes zera o gasto quando o mês (UTC) muda. Chamar com gastoMu travado.
func (s *Servidor) virarMes() {
	if mes := time.Now().UTC().Format("2006-01"); mes != s.gastoMes {
		s.gastoMes, s.gasto = mes, map[int64]int64{}
	}
}

const cookieAdmin = "comporta_admin"

// O cookie guarda o SHA-256 do token, não o token: um cookie vazado não serve como Bearer.
func (s *Servidor) valorCookie() string {
	h := sha256.Sum256([]byte("cookie:" + s.d.AdminToken))
	return hex.EncodeToString(h[:])
}

func iguais(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

func (s *Servidor) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok := false
		if t, temBearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); temBearer {
			ok = iguais(t, s.d.AdminToken)
		} else if c, err := r.Cookie(cookieAdmin); err == nil {
			ok = iguais(c.Value, s.valorCookie())
		}
		if s.d.AdminToken == "" || !ok {
			erro(w, http.StatusUnauthorized, "nao_autorizado", "login de admin necessário")
			return
		}
		h(w, r)
	}
}
