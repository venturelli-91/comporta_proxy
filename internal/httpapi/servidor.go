// Package httpapi expõe a API compatível com OpenAI (/v1), a API do painel (/admin),
// as sondas de saúde e o próprio painel embutido.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"

	"github.com/venturelli-91/comporta_proxy/internal/armazenamento"
	"github.com/venturelli-91/comporta_proxy/internal/cache"
	"github.com/venturelli-91/comporta_proxy/internal/gateway"
)

type Deps struct {
	Config     *gateway.Config
	Roteador   *gateway.Roteador
	Banco      *armazenamento.Banco
	Gravador   *armazenamento.Gravador
	Cache      *cache.Cache
	AdminToken string
	Painel     fs.FS // nil = sem painel
	Log        *slog.Logger
}

// snapshot é tudo que a autenticação lê em cada requisição. É trocado inteiro (atomic.Pointer)
// a cada mutação do admin: leitura sem lock, e uma chave revogada deixa de valer na hora.
type snapshot struct {
	chaves   map[string]int64 // hash → cliente
	clientes map[int64]armazenamento.Cliente
}

type Servidor struct {
	d    Deps
	snap atomic.Pointer[snapshot]
	sf   singleflight.Group
	met  contadores
	bg   sync.WaitGroup // shadows em andamento
	// sombras limita os shadows simultâneos: tráfego de comparação não pode disputar o limite
	// de concorrência da conta com o tráfego real. Sem vaga, o shadow é descartado e contado.
	sombras chan struct{}

	limMu   sync.Mutex
	limites map[int64]limitador
	// emCurso conta as requisições abertas por cliente. Com porCliente abaixo da concorrência
	// do modelo, um cliente sozinho (ou com streams longos) não esgota as vagas de todos.
	emCurso    map[int64]int
	porCliente int

	gastoMu  sync.Mutex
	gastoMes string
	gasto    map[int64]int64

	primeiroToken, entreChunks, total time.Duration
}

type limitador struct {
	rpm int
	l   *rate.Limiter
}

func Novo(d Deps) (*Servidor, error) {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	t := d.Config.Timeouts
	s := &Servidor{
		d: d, limites: map[int64]limitador{}, sombras: make(chan struct{}, 16),
		emCurso: map[int64]int{}, porCliente: 32,
		primeiroToken: padrao(t.PrimeiroToken(), 30*time.Second),
		entreChunks:   padrao(t.EntreChunks(), 60*time.Second),
		total:         padrao(t.Total(), 10*time.Minute),
	}
	ctx := context.Background()
	if err := s.Recarregar(ctx); err != nil {
		return nil, err
	}
	// O orçamento vive em memória (somado a cada chamada) e parte do que já está no banco.
	// ponytail: uma instância só; com várias, o gasto precisa ir para o Redis.
	agora := time.Now().UTC()
	gasto, err := d.Banco.GastoDoMes(ctx, agora)
	if err != nil {
		return nil, err
	}
	s.gasto, s.gastoMes = gasto, agora.Format("2006-01")
	return s, nil
}

func padrao(d, p time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return p
}

// Recarregar relê clientes e chaves e troca o snapshot.
func (s *Servidor) Recarregar(ctx context.Context) error {
	clientes, err := s.d.Banco.ListarClientes(ctx)
	if err != nil {
		return err
	}
	chaves, err := s.d.Banco.ChavesAtivas(ctx)
	if err != nil {
		return err
	}
	sn := &snapshot{chaves: chaves, clientes: map[int64]armazenamento.Cliente{}}
	for _, c := range clientes {
		sn.clientes[c.ID] = c
	}
	s.snap.Store(sn)
	return nil
}

// Esperar aguarda as chamadas shadow em andamento (shutdown gracioso e testes).
func (s *Servidor) Esperar() { s.bg.Wait() }

func (s *Servidor) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", s.autenticar(s.chat))
	mux.HandleFunc("GET /v1/models", s.autenticar(s.modelos))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /readyz", s.pronto)
	mux.HandleFunc("GET /metrics", s.metricas)
	s.rotasAdmin(mux)
	if s.d.Painel != nil {
		mux.Handle("/", painel(s.d.Painel))
	}
	return s.base(mux)
}

func (s *Servidor) pronto(w http.ResponseWriter, r *http.Request) {
	if err := s.d.Banco.Ping(r.Context()); err != nil {
		erro(w, http.StatusServiceUnavailable, "indisponivel", "banco indisponível")
		return
	}
	w.Write([]byte("ok"))
}

type chaveCtx int

const (
	ctxRequestID chaveCtx = iota
	ctxCliente
)

// registro guarda o status para o log; Unwrap deixa o http.ResponseController achar o Flush.
type registro struct {
	http.ResponseWriter
	status int
}

func (r *registro) WriteHeader(s int) {
	if r.status == 0 {
		r.status = s
	}
	r.ResponseWriter.WriteHeader(s)
}

func (r *registro) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (r *registro) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// base: request ID, recover e uma linha de log por requisição (sem conteúdo de mensagem).
func (s *Servidor) base(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" || len(id) > 64 {
			id = rand.Text()[:16]
		}
		w.Header().Set("X-Request-Id", id)
		reg := &registro{ResponseWriter: w}
		inicio := time.Now()
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				s.d.Log.Error("panic", "request_id", id, "erro", fmt.Sprint(p))
				if reg.status == 0 {
					erro(reg, http.StatusInternalServerError, "interno", "erro interno")
				}
			}
			s.d.Log.Info("http", "request_id", id, "metodo", r.Method, "caminho", r.URL.Path,
				"status", reg.status, "ms", time.Since(inicio).Milliseconds())
		}()
		h.ServeHTTP(reg, r.WithContext(context.WithValue(r.Context(), ctxRequestID, id)))
	})
}

func erro(w http.ResponseWriter, status int, codigo, mensagem string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"erro": map[string]string{"codigo": codigo, "mensagem": mensagem}})
}

func responderJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// painel serve o build do React; rotas desconhecidas caem no index.html (roteamento do lado do cliente).
func painel(dist fs.FS) http.Handler {
	arquivos := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caminho := strings.TrimPrefix(r.URL.Path, "/")
		if _, err := fs.Stat(dist, caminho); caminho != "" && err != nil {
			http.ServeFileFS(w, r, dist, "index.html")
			return
		}
		arquivos.ServeHTTP(w, r)
	})
}
