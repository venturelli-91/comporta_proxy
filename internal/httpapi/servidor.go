// Package httpapi exposes the OpenAI-compatible API (/v1), the dashboard API (/admin),
// the health probes and the embedded dashboard itself.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
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
	Painel     fs.FS // nil = no dashboard
	Log        *slog.Logger
	// ConfiarProxy (trust proxy): there is a proxy in front (Cloud Run) that appends the real IP to
	// X-Forwarded-For. Without it, the header belongs to the client and identifies nobody.
	ConfiarProxy bool
}

// snapshot is everything authentication reads on each request. It's swapped whole (atomic.Pointer)
// on every admin mutation: lock-free reads, and a revoked key stops working immediately.
type snapshot struct {
	chaves   map[string]int64 // hash → client
	clientes map[int64]armazenamento.Cliente
}

type Servidor struct {
	d    Deps
	snap atomic.Pointer[snapshot]
	// recMu serializes Recarregar; antesDeTrocar is a test-only hook (nil in production).
	recMu         sync.Mutex
	antesDeTrocar func()
	sf            singleflight.Group
	met           contadores
	bg            sync.WaitGroup // shadows in flight
	ativos        sync.WaitGroup // requests in flight (so shutdown waits for their usage records)
	// sombras bounds concurrent shadows: comparison traffic must not compete with real traffic for
	// the account's concurrency limit. Without a free slot, the shadow is dropped and counted.
	sombras chan struct{}
	// ctxSombras is the shadows' context, independent of the request (the client leaving doesn't
	// cancel them) and cancelled on shutdown when the deadline runs out.
	ctxSombras   context.Context
	pararSombras context.CancelFunc

	limMu   sync.Mutex
	limites map[int64]*rate.Limiter
	// emCurso counts open requests per client. With porCliente below the model's concurrency,
	// a single client (or one with long streams) can't use up everyone's slots.
	emCurso    map[int64]int
	porCliente int

	sessMu     sync.Mutex
	sessoes    map[string]time.Time     // cookie hash → expiry
	tentativas map[string]*rate.Limiter // login attempts per IP

	gastoMu  sync.Mutex
	gastoMes string
	gasto    map[int64]int64

	primeiroToken, entreChunks, total time.Duration
}

func Novo(d Deps) (*Servidor, error) {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	// An empty token would open the admin: ConstantTimeCompare("", "") returns 1. Refused here, once,
	// instead of being checked on every login.
	if d.AdminToken == "" {
		return nil, errors.New("httpapi: empty AdminToken")
	}
	t := d.Config.Timeouts
	s := &Servidor{
		d: d, limites: map[int64]*rate.Limiter{}, sombras: make(chan struct{}, 16),
		emCurso: map[int64]int{}, porCliente: 32,
		sessoes: map[string]time.Time{}, tentativas: map[string]*rate.Limiter{},
		primeiroToken: t.PrimeiroToken(), entreChunks: t.EntreChunks(), total: t.Total(),
	}
	s.ctxSombras, s.pararSombras = context.WithCancel(context.Background())
	ctx := context.Background()
	if err := s.Recarregar(ctx); err != nil {
		return nil, err
	}
	// The budget lives in memory (added up on every call) and starts from what's already in the database.
	// ponytail: a single instance; with several, the spend has to move to Redis.
	agora := time.Now().UTC()
	gasto, err := d.Banco.GastoDoMes(ctx, agora)
	if err != nil {
		return nil, err
	}
	s.gasto, s.gastoMes = gasto, agora.Format("2006-01")
	return s, nil
}

// Recarregar (reload) re-reads clients and keys and swaps the snapshot. Reading and swapping happen
// under the same lock: otherwise a Recarregar that read before a revocation could store its snapshot
// after one that read after it, and the revoked key would work again.
func (s *Servidor) Recarregar(ctx context.Context) error {
	s.recMu.Lock()
	defer s.recMu.Unlock()
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
	if s.antesDeTrocar != nil {
		s.antesDeTrocar()
	}
	s.snap.Store(sn)
	return nil
}

// Encerrar (shut down) waits for in-flight requests and then shadows until ctx's deadline. Past the
// deadline, it cancels the remaining shadows and waits for all of them to record usage (they exit
// right after the cancellation). Requests come first: only they start shadows, so after them no
// bg.Go runs concurrently with bg.Wait. The caller cuts the connections beforehand (h.Close).
func (s *Servidor) Encerrar(ctx context.Context) {
	fim := make(chan struct{})
	go func() { s.ativos.Wait(); s.bg.Wait(); close(fim) }()
	select {
	case <-fim:
	case <-ctx.Done():
		s.pararSombras()
		<-fim
	}
}

func (s *Servidor) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", s.autenticar(s.chat))
	mux.HandleFunc("GET /v1/models", s.autenticar(s.modelos))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /readyz", s.pronto)
	mux.HandleFunc("GET /metrics", s.metricas)
	s.rotasAdmin(mux)
	// More specific than "/": a wrong API route gets a JSON 404, not the dashboard's index.html.
	naoExiste := func(w http.ResponseWriter, r *http.Request) {
		erro(w, http.StatusNotFound, "rota_inexistente", "route not found")
	}
	mux.HandleFunc("/v1/", naoExiste)
	mux.HandleFunc("/admin/", naoExiste)
	if s.d.Painel != nil {
		mux.Handle("/", painel(s.d.Painel))
	}
	return s.base(mux)
}

func (s *Servidor) pronto(w http.ResponseWriter, r *http.Request) {
	if err := s.d.Banco.Ping(r.Context()); err != nil {
		erro(w, http.StatusServiceUnavailable, "indisponivel", "database unavailable")
		return
	}
	w.Write([]byte("ok"))
}

type chaveCtx int

const (
	ctxRequestID chaveCtx = iota
	ctxCliente
)

// registro keeps the status for the log; Unwrap lets http.ResponseController find Flush.
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

// base: request ID, recover and one log line per request (no message content).
func (s *Servidor) base(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" || len(id) > 64 {
			id = rand.Text()[:16]
		}
		w.Header().Set("X-Request-Id", id)
		s.ativos.Add(1)
		defer s.ativos.Done()
		reg := &registro{ResponseWriter: w}
		inicio := time.Now()
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				s.d.Log.Error("panic", "request_id", id, "error", fmt.Sprint(p))
				if reg.status == 0 {
					erro(reg, http.StatusInternalServerError, "interno", "internal error")
				}
			}
			s.d.Log.Info("http", "request_id", id, "method", r.Method, "path", r.URL.Path,
				"status", reg.status, "ms", time.Since(inicio).Milliseconds())
		}()
		h.ServeHTTP(reg, r.WithContext(context.WithValue(r.Context(), ctxRequestID, id)))
	})
}

func erro(w http.ResponseWriter, status int, codigo, mensagem string) {
	responderJSON(w, status, map[string]any{"erro": map[string]string{"codigo": codigo, "mensagem": mensagem}})
}

func responderJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// painel serves the React build; unknown routes fall back to index.html (client-side routing).
// index.html goes out with no-cache: it's the only build file without a hash and it points at the
// hashed bundle, so a cached copy would keep loading the old bundle after a deploy. The embedded
// files have no modification time, so without this the browser caches it heuristically.
func painel(dist fs.FS) http.Handler {
	arquivos := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caminho := strings.TrimPrefix(r.URL.Path, "/")
		_, err := fs.Stat(dist, caminho)
		if caminho == "" || err != nil {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if caminho != "" && err != nil {
			http.ServeFileFS(w, r, dist, "index.html")
			return
		}
		arquivos.ServeHTTP(w, r)
	})
}
