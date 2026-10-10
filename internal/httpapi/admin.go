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
	lim := s.limiteLogin(ipDe(r))
	if lim.Tokens() < 1 {
		retryAfter(w, time.Minute)
		erro(w, http.StatusTooManyRequests, "muitas_tentativas", "muitas tentativas de login, aguarde um minuto")
		return
	}
	var v struct{ Token string }
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&v)
	if s.d.AdminToken == "" || !iguais(v.Token, s.d.AdminToken) {
		lim.Allow() // só tentativa errada consome
		erro(w, http.StatusUnauthorized, "nao_autorizado", "token de admin inválido")
		return
	}
	// Atrás do TLS do Cloud Run a requisição chega em HTTP puro, com X-Forwarded-Proto.
	seguro := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{Name: cookieAdmin, Value: s.novaSessao(), Path: "/", MaxAge: int(duracaoSessao.Seconds()),
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: seguro})
	w.WriteHeader(http.StatusNoContent)
}

// limiteLogin: 5 tentativas erradas por IP, uma nova a cada 12 s. Por IP para que um
// atacante não tranque o admin de verdade do lado de fora.
// ponytail: mapa zerado ao passar de 10 mil IPs; LRU se isso virar alvo.
func (s *Servidor) limiteLogin(ip string) *rate.Limiter {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	if len(s.tentativas) > 10_000 {
		s.tentativas = map[string]*rate.Limiter{}
	}
	l, ok := s.tentativas[ip]
	if !ok {
		l = rate.NewLimiter(rate.Every(12*time.Second), 5)
		s.tentativas[ip] = l
	}
	return l
}

// ipDe usa a última entrada do X-Forwarded-For, que é a que o proxy confiável (o front-end do
// Cloud Run) acrescenta; o cliente só consegue forjar as anteriores. Sem proxy na frente, o
// cabeçalho inteiro é forjável e o limite vira só um obstáculo: a proteção real é o token
// ter entropia alta (o gerado pelo binário tem 128 bits).
func ipDe(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		partes := strings.Split(xff, ",")
		return strings.TrimSpace(partes[len(partes)-1])
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
		erro(w, http.StatusBadRequest, "id_invalido", nome+" inválido")
	}
	return id, err == nil
}

func (s *Servidor) lerCliente(w http.ResponseWriter, r *http.Request) (armazenamento.Cliente, bool) {
	var c armazenamento.Cliente
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&c); err != nil || c.Nome == "" {
		erro(w, http.StatusBadRequest, "cliente_invalido", "informe ao menos o nome do cliente")
		return c, false
	}
	for _, m := range c.Modelos {
		if _, ok := s.d.Config.Modelos[m]; !ok {
			erro(w, http.StatusBadRequest, "modelo_desconhecido", "modelo "+m+" não existe")
			return c, false
		}
	}
	return c, true
}

// mutou relê o snapshot depois de qualquer alteração e responde. Um erro do banco vira 500
// sem detalhe; "não encontrado" vira 404.
func (s *Servidor) mutou(w http.ResponseWriter, r *http.Request, err error, status int, v any) {
	if errors.Is(err, armazenamento.ErrNaoEncontrado) {
		erro(w, http.StatusNotFound, "nao_encontrado", "registro não encontrado")
		return
	}
	if err == nil {
		err = s.Recarregar(r.Context())
	}
	if err != nil {
		s.d.Log.Error("admin", "erro", err.Error())
		erro(w, http.StatusInternalServerError, "interno", "erro interno")
		return
	}
	if v == nil {
		w.WriteHeader(status)
		return
	}
	responderJSON(w, status, v)
}

func (s *Servidor) listarClientes(w http.ResponseWriter, r *http.Request) {
	lista, err := s.d.Banco.ListarClientes(r.Context())
	if err != nil {
		s.mutou(w, r, err, 0, nil)
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
		s.mutou(w, r, err, 0, nil)
		return
	}
	responderJSON(w, http.StatusOK, lista)
}

// criarChave é o único momento em que a chave inteira aparece.
func (s *Servidor) criarChave(w http.ResponseWriter, r *http.Request) {
	id, ok := idDe(w, r, "id")
	if !ok {
		return
	}
	if _, existe := s.snap.Load().clientes[id]; !existe {
		erro(w, http.StatusNotFound, "nao_encontrado", "cliente não encontrado")
		return
	}
	texto, k, err := s.d.Banco.CriarChave(r.Context(), id)
	s.mutou(w, r, err, http.StatusCreated, map[string]any{"chave": texto, "info": k})
}

func (s *Servidor) revogarChave(w http.ResponseWriter, r *http.Request) {
	if id, ok := idDe(w, r, "chave"); ok {
		s.mutou(w, r, s.d.Banco.RevogarChave(r.Context(), id), http.StatusNoContent, nil)
	}
}

// uso: /admin/uso?de=RFC3339&ate=RFC3339&cliente=ID (padrão: últimas 24 h, todos).
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
		s.mutou(w, r, err, 0, nil)
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
