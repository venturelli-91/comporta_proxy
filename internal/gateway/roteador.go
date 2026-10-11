package gateway

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/venturelli-91/comporta_proxy/internal/provedor"
	"github.com/venturelli-91/comporta_proxy/internal/resiliencia"
)

var (
	// ErrSemDestino: every destination of the alias has its breaker open (becomes 503).
	ErrSemDestino = errors.New("no destination available")
	// ErrOcupado: the alias's semaphore stayed full past the short wait (becomes 429).
	ErrOcupado = errors.New("model busy")
)

// Roteador (router) takes an alias ("rapido") to a real provider, protecting each destination with
// a circuit breaker and retry, and the whole alias with a semaphore below the account's limit.
//
// The breaker is per client and destination. Shared, any failure a client could provoke
// (generations that exceed the deadline, requests that make the provider hang) would open the
// destination for everyone. Isolated, each client only takes down its own path, and the breaker can
// count timeouts without fear. The cost: when the provider is really sick, each client needs
// its own failures for its breaker to open.
type Roteador struct {
	cfg      *Config
	clientes map[string]*provedor.Cliente
	semaforo map[string]chan struct{} // per alias

	brMu     sync.Mutex
	breakers map[chaveBreaker]*gobreaker.CircuitBreaker[*http.Response]

	baseRetry      time.Duration
	esperaSemaforo time.Duration
}

func NovoRoteador(cfg *Config, clientes map[string]*provedor.Cliente) *Roteador {
	r := &Roteador{
		cfg: cfg, clientes: clientes,
		breakers:       map[chaveBreaker]*gobreaker.CircuitBreaker[*http.Response]{},
		semaforo:       map[string]chan struct{}{},
		baseRetry:      200 * time.Millisecond,
		esperaSemaforo: 2 * time.Second,
	}
	for alias, m := range cfg.Modelos {
		r.semaforo[alias] = make(chan struct{}, max(m.Concorrencia, 1))
	}
	return r
}

type chaveBreaker struct {
	cliente int64
	destino string
}

// breaker returns (creating it the first time) the client's breaker for the destination.
// ponytail: a removed client's breakers stay in the map (clients × destinations, IDs never reused).
// An Esquecer(cliente) called by the admin solves it if this grows.
func (r *Roteador) breaker(cliente int64, d Destino) *gobreaker.CircuitBreaker[*http.Response] {
	k := chaveBreaker{cliente, d.String()}
	r.brMu.Lock()
	defer r.brMu.Unlock()
	b, ok := r.breakers[k]
	if !ok {
		b = novoBreaker(d.String())
		r.breakers[k] = b
	}
	return b
}

func novoBreaker(nome string) *gobreaker.CircuitBreaker[*http.Response] {
	return gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
		Name:        nome,
		MaxRequests: 1,
		Interval:    time.Minute,
		Timeout:     10 * time.Second,
		// Opens with half the calls failing, but only after a minimum sample:
		// a single early failure must not take the destination down.
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.Requests >= 5 && c.TotalFailures*2 >= c.Requests
		},
		// A malformed request (400, 413, 422) doesn't mean the provider is sick.
		IsSuccessful: func(err error) bool { return err == nil || erroDoPedido(err) },
		// A client that gave up says nothing about the provider's health. Exceeded deadlines and watchdog
		// cuts do count: since the breaker belongs to the client, it only takes down its own path.
		IsExcluded: func(err error) bool { return errors.Is(err, context.Canceled) },
	})
}

// erroDoPedido (request error): the provider rejected the request itself; it would fail the same way
// on any destination. 401/402/403 are left out on purpose: they're a problem with the gateway's account
// at the provider (invalid key, no balance) and the next destination may serve it.
func erroDoPedido(err error) bool {
	var e *provedor.ErroHTTP
	return errors.As(err, &e) && (e.Status == 400 || e.Status == 413 || e.Status == 422)
}

// Chamar (call) tries the alias's destinations in order. It returns the open 2xx response: the caller
// reads the body (streaming or not) and closes it; closing the body also releases the semaphore.
func (r *Roteador) Chamar(ctx context.Context, cliente int64, alias string, corpo map[string]json.RawMessage) (*http.Response, Destino, error) {
	m, ok := r.cfg.Modelos[alias]
	if !ok {
		return nil, Destino{}, fmt.Errorf("alias %q: %w", alias, ErrSemDestino)
	}
	sem := r.semaforo[alias]
	espera := time.NewTimer(r.esperaSemaforo)
	defer espera.Stop()
	select {
	case sem <- struct{}{}:
	case <-espera.C:
		return nil, Destino{}, ErrOcupado
	case <-ctx.Done():
		return nil, Destino{}, ctx.Err()
	}
	liberar := sync.OnceFunc(func() { <-sem })

	var ultimo error
	for _, d := range m.Destinos {
		resp, err := r.ChamarDestino(ctx, cliente, d, corpo)
		if err == nil {
			resp.Body = &corpoComFim{ReadCloser: resp.Body, fim: liberar}
			return resp, d, nil
		}
		ultimo = err
		// A malformed request would fail the same way on the next destination; a dead context has nobody to answer.
		if erroDoPedido(err) || ctx.Err() != nil {
			break
		}
	}
	liberar()
	if errors.Is(ultimo, gobreaker.ErrOpenState) || errors.Is(ultimo, gobreaker.ErrTooManyRequests) {
		return nil, Destino{}, ErrSemDestino
	}
	return nil, Destino{}, ultimo
}

// ChamarDestino calls a specific destination (also used by shadow mode), with the retry
// inside the breaker: the breaker counts only the final result of each call.
func (r *Roteador) ChamarDestino(ctx context.Context, cliente int64, d Destino, corpo map[string]json.RawMessage) (*http.Response, error) {
	c := maps.Clone(corpo)
	c["model"], _ = json.Marshal(d.Modelo)
	b, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("encoding body: %w", err)
	}
	prov := r.clientes[d.Provedor]
	return r.breaker(cliente, d).Execute(func() (*http.Response, error) {
		var resp *http.Response
		err := resiliencia.Retry(ctx, 3, r.baseRetry, func() (err error) {
			resp, err = prov.Enviar(ctx, b)
			return err
		})
		return resp, err
	})
}

type EstadoDestino struct {
	Destino string `json:"destino"`
	Breaker string `json:"breaker"` // worst state across clients
	Abertos int    `json:"abertos"` // clients whose breaker isn't closed
}

// Estado (state) summarizes the breakers per destination, alphabetically, for the dashboard.
// ponytail: no semaphore occupancy; it comes in together with Prometheus metrics.
func (r *Roteador) Estado() []EstadoDestino {
	por := map[string]*EstadoDestino{}
	for _, m := range r.cfg.Modelos {
		for _, d := range m.todos() {
			por[d.String()] = &EstadoDestino{Destino: d.String(), Breaker: gobreaker.StateClosed.String()}
		}
	}
	r.brMu.Lock()
	for k, b := range r.breakers {
		e := por[k.destino]
		if st := b.State(); st != gobreaker.StateClosed {
			e.Abertos++
			if st == gobreaker.StateOpen || e.Breaker == gobreaker.StateClosed.String() {
				e.Breaker = st.String()
			}
		}
	}
	r.brMu.Unlock()
	out := make([]EstadoDestino, 0, len(por))
	for _, e := range por {
		out = append(out, *e)
	}
	slices.SortFunc(out, func(a, b EstadoDestino) int { return cmp.Compare(a.Destino, b.Destino) })
	return out
}

// corpoComFim runs `fim` once when the body is closed.
type corpoComFim struct {
	io.ReadCloser
	fim func()
}

func (c *corpoComFim) Close() error {
	err := c.ReadCloser.Close()
	c.fim()
	return err
}
