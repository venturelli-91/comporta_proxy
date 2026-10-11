package httpapi

import (
	"encoding/json"
	"errors"
	"maps"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/venturelli-91/comporta_proxy/internal/armazenamento"
)

func (s *Servidor) rotasAdmin(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/login", s.login)
	mux.HandleFunc("POST /admin/logout", s.logout)
	mux.HandleFunc("GET /admin/sessao", s.admin(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	mux.HandleFunc("GET /admin/clientes", s.admin(s.listarClientes))
	mux.HandleFunc("POST /admin/clientes", s.admin(s.criarCliente))
	mux.HandleFunc("PUT /admin/clientes/{id}", s.admin(s.atualizarCliente))
	mux.HandleFunc("DELETE /admin/clientes/{id}", s.admin(s.removerCliente))
	mux.HandleFunc("GET /admin/clientes/{id}/chaves", s.admin(s.listarChaves))
	mux.HandleFunc("POST /admin/clientes/{id}/chaves", s.admin(s.criarChave))
	mux.HandleFunc("DELETE /admin/clientes/{id}/chaves/{chave}", s.admin(s.revogarChave))
	mux.HandleFunc("GET /admin/uso", s.admin(s.uso))
	mux.HandleFunc("GET /admin/provedores", s.admin(s.provedores))
	mux.HandleFunc("GET /admin/modelos", s.admin(s.modelosAdmin))
}

func (s *Servidor) login(w http.ResponseWriter, r *http.Request) {
	// Allow() checks and consumes in a single step: parallel attempts can't all slip past the check.
	// A correct login also spends a token; since the limit is per IP, an attacker can't lock the admin out.
	if !s.limiteLogin(s.ipDe(r)).Allow() {
		retryAfter(w, time.Minute)
		erro(w, http.StatusTooManyRequests, "muitas_tentativas", "too many login attempts, wait a minute")
		return
	}
	var v struct{ Token string }
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&v)
	if !iguais(v.Token, s.d.AdminToken) {
		erro(w, http.StatusUnauthorized, "nao_autorizado", "invalid admin token")
		return
	}
	// Behind Cloud Run's TLS the request arrives as plain HTTP, with X-Forwarded-Proto.
	seguro := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{Name: cookieAdmin, Value: s.novaSessao(), Path: "/", MaxAge: int(duracaoSessao.Seconds()),
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: seguro})
	w.WriteHeader(http.StatusNoContent)
}

// limiteLogin: 5 attempts per IP, a new one every 12 s. Per IP so that an attacker
// can't lock the real admin out from outside.
// ponytail: pruning scans the map and only removes full limiters; with 10k IPs spent at once the
// map grows and each login becomes O(n). A bounded LRU if this becomes a target.
func (s *Servidor) limiteLogin(ip string) *rate.Limiter {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	if len(s.tentativas) > 10_000 {
		for k, l := range s.tentativas {
			if l.Tokens() >= float64(l.Burst()) { // full = same as a new one, holds nothing
				delete(s.tentativas, k)
			}
		}
	}
	l, ok := s.tentativas[ip]
	if !ok {
		l = rate.NewLimiter(rate.Every(12*time.Second), 5)
		s.tentativas[ip] = l
	}
	return l
}

// ipDe uses the last X-Forwarded-For entry, the one the trusted proxy (Cloud Run's front end)
// appends; the client can only forge the earlier ones. Without that proxy the whole header belongs
// to the client, who would get a fresh bucket on every attempt: then the connection address is used.
// ponytail: assumes a single proxy (Cloud Run directly, Render). With an HTTPS Load Balancer in front,
// the last entry would be the LB's IP and everyone would share one bucket: count N trusted proxies from the end.
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

func (s *Servidor) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieAdmin); err == nil {
		s.encerrarSessao(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieAdmin, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func idDe(w http.ResponseWriter, r *http.Request, nome string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(nome), 10, 64)
	if err != nil {
		erro(w, http.StatusBadRequest, "id_invalido", "invalid "+nome)
	}
	return id, err == nil
}

func (s *Servidor) lerCliente(w http.ResponseWriter, r *http.Request) (armazenamento.Cliente, bool) {
	var c armazenamento.Cliente
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&c); err != nil || c.Nome == "" {
		erro(w, http.StatusBadRequest, "cliente_invalido", "the client needs at least a name")
		return c, false
	}
	for _, m := range c.Modelos {
		if _, ok := s.d.Config.Modelos[m]; !ok {
			erro(w, http.StatusBadRequest, "modelo_desconhecido", "model "+m+" does not exist")
			return c, false
		}
	}
	return c, true
}

// mutou reloads the snapshot after any change and responds. A database error becomes a 500
// without details; "not found" becomes a 404.
func (s *Servidor) mutou(w http.ResponseWriter, r *http.Request, err error, status int, v any) {
	if errors.Is(err, armazenamento.ErrNaoEncontrado) {
		erro(w, http.StatusNotFound, "nao_encontrado", "record not found")
		return
	}
	if err == nil {
		err = s.Recarregar(r.Context())
	}
	if err != nil {
		s.interno(w, err)
		return
	}
	if v == nil {
		w.WriteHeader(status)
		return
	}
	responderJSON(w, status, v)
}

// interno responds 500 without details; the real error goes only to the log.
func (s *Servidor) interno(w http.ResponseWriter, err error) {
	s.d.Log.Error("admin", "error", err.Error())
	erro(w, http.StatusInternalServerError, "interno", "internal error")
}

func (s *Servidor) listarClientes(w http.ResponseWriter, r *http.Request) {
	lista, err := s.d.Banco.ListarClientes(r.Context())
	if err != nil {
		s.interno(w, err)
		return
	}
	responderJSON(w, http.StatusOK, lista)
}

func (s *Servidor) criarCliente(w http.ResponseWriter, r *http.Request) {
	if c, ok := s.lerCliente(w, r); ok {
		c, err := s.d.Banco.CriarCliente(r.Context(), c)
		s.mutou(w, r, err, http.StatusCreated, c)
	}
}

func (s *Servidor) atualizarCliente(w http.ResponseWriter, r *http.Request) {
	id, ok := idDe(w, r, "id")
	if !ok {
		return
	}
	if c, ok := s.lerCliente(w, r); ok {
		c.ID = id
		s.mutou(w, r, s.d.Banco.AtualizarCliente(r.Context(), c), http.StatusOK, c)
	}
}

func (s *Servidor) removerCliente(w http.ResponseWriter, r *http.Request) {
	if id, ok := idDe(w, r, "id"); ok {
		s.mutou(w, r, s.d.Banco.RemoverCliente(r.Context(), id), http.StatusNoContent, nil)
	}
}

func (s *Servidor) listarChaves(w http.ResponseWriter, r *http.Request) {
	id, ok := idDe(w, r, "id")
	if !ok {
		return
	}
	lista, err := s.d.Banco.ListarChaves(r.Context(), id)
	if err != nil {
		s.interno(w, err)
		return
	}
	responderJSON(w, http.StatusOK, lista)
}

// criarChave (create key) is the only moment the full key appears.
func (s *Servidor) criarChave(w http.ResponseWriter, r *http.Request) {
	id, ok := idDe(w, r, "id")
	if !ok {
		return
	}
	if _, existe := s.snap.Load().clientes[id]; !existe {
		erro(w, http.StatusNotFound, "nao_encontrado", "client not found")
		return
	}
	texto, k, err := s.d.Banco.CriarChave(r.Context(), id)
	s.mutou(w, r, err, http.StatusCreated, map[string]any{"chave": texto, "info": k})
}

func (s *Servidor) revogarChave(w http.ResponseWriter, r *http.Request) {
	cliente, ok := idDe(w, r, "id")
	if !ok {
		return
	}
	if id, ok := idDe(w, r, "chave"); ok {
		s.mutou(w, r, s.d.Banco.RevogarChave(r.Context(), cliente, id), http.StatusNoContent, nil)
	}
}

// uso (usage): /admin/uso?de=RFC3339&ate=RFC3339&cliente=ID (default: last 24 h, all clients).
func (s *Servidor) uso(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ate, err := time.Parse(time.RFC3339, q.Get("ate"))
	if err != nil {
		ate = time.Now()
	}
	de, err := time.Parse(time.RFC3339, q.Get("de"))
	if err != nil {
		de = ate.Add(-24 * time.Hour)
	}
	cliente, _ := strconv.ParseInt(q.Get("cliente"), 10, 64)
	rel, err := s.d.Banco.Relatorio(r.Context(), de, ate, cliente)
	if err != nil {
		s.interno(w, err)
		return
	}
	responderJSON(w, http.StatusOK, rel)
}

func (s *Servidor) provedores(w http.ResponseWriter, r *http.Request) {
	responderJSON(w, http.StatusOK, s.d.Roteador.Estado())
}

func (s *Servidor) modelosAdmin(w http.ResponseWriter, r *http.Request) {
	type modelo struct {
		Alias    string   `json:"alias"`
		Destinos []string `json:"destinos"`
		Shadow   string   `json:"shadow,omitempty"`
	}
	out := []modelo{}
	for _, alias := range slices.Sorted(maps.Keys(s.d.Config.Modelos)) {
		m := s.d.Config.Modelos[alias]
		mo := modelo{Alias: alias}
		for _, d := range m.Destinos {
			mo.Destinos = append(mo.Destinos, d.String())
		}
		if m.Shadow != nil {
			mo.Shadow = m.Shadow.String()
		}
		out = append(out, mo)
	}
	responderJSON(w, http.StatusOK, out)
}
