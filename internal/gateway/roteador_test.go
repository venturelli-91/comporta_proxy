package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/venturelli-91/comporta_proxy/internal/provedor"
)

func corpoChat() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"model":    json.RawMessage(`"rapido"`),
		"messages": json.RawMessage(`[{"role":"user","content":"oi"}]`),
	}
}

// novoCenario starts one HTTP server per provider and a router with the alias "rapido" → [a, b].
func novoCenario(t *testing.T, a, b http.Handler, concorrencia int) *Roteador {
	t.Helper()
	cfg := &Config{
		Provedores: map[string]ProvedorCfg{"a": {}, "b": {}},
		Modelos: map[string]ModeloCfg{"rapido": {
			Destinos:     []Destino{{"a", "modelo-a"}, {"b", "modelo-b"}},
			Concorrencia: concorrencia,
		}},
	}
	clientes := map[string]*provedor.Cliente{}
	for nome, h := range map[string]http.Handler{"a": a, "b": b} {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		clientes[nome] = provedor.Novo(srv.URL, "", 10, time.Second)
	}
	r := NovoRoteador(cfg, clientes)
	r.baseRetry = time.Millisecond
	return r
}

func chamar(t *testing.T, r *Roteador) (Destino, error) {
	t.Helper()
	resp, d, err := r.Chamar(context.Background(), 1, "rapido", corpoChat())
	if err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return d, err
}

func TestFallbackQuandoPrincipalFalha(t *testing.T) {
	a, b := &provedor.Falso{TaxaErro: 1}, &provedor.Falso{}
	r := novoCenario(t, a, b, 10)
	d, err := chamar(t, r)
	if err != nil {
		t.Fatal(err)
	}
	if d.Provedor != "b" || d.Modelo != "modelo-b" {
		t.Fatalf("destination = %v, want b/modelo-b", d)
	}
	if a.Chamadas.Load() != 3 {
		t.Errorf("primary got %d calls, want 3 (1 + 2 retries)", a.Chamadas.Load())
	}
}

func TestErroDoClienteNaoFazFallback(t *testing.T) {
	a := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid parameter"}}`))
	})
	b := &provedor.Falso{}
	r := novoCenario(t, a, b, 10)
	_, err := chamar(t, r)
	var e *provedor.ErroHTTP
	if !errors.As(err, &e) || e.Status != 400 {
		t.Fatalf("error = %v, want the provider's 400", err)
	}
	if b.Chamadas.Load() != 0 {
		t.Fatal("400 is a client error: it should not try the fallback")
	}
}

func TestBreakerAbreEPulaODestino(t *testing.T) {
	a, b := &provedor.Falso{TaxaErro: 1}, &provedor.Falso{}
	r := novoCenario(t, a, b, 10)
	for range 5 {
		if _, err := chamar(t, r); err != nil {
			t.Fatal(err)
		}
	}
	antes := a.Chamadas.Load()
	if _, err := chamar(t, r); err != nil {
		t.Fatal(err)
	}
	if a.Chamadas.Load() != antes {
		t.Fatal("with the breaker open, the primary should not be called")
	}
	if e := r.Estado(); e[0].Destino != "a/modelo-a" || e[0].Breaker != "open" {
		t.Fatalf("state = %+v", e)
	}
}

func TestSemDestinoQuandoTodosOsBreakersAbrem(t *testing.T) {
	r := novoCenario(t, &provedor.Falso{TaxaErro: 1}, &provedor.Falso{TaxaErro: 1}, 10)
	var err error
	for range 6 {
		_, err = chamar(t, r)
	}
	if !errors.Is(err, ErrSemDestino) {
		t.Fatalf("error = %v, want ErrSemDestino", err)
	}
}

// contaConcorrencia measures how many requests the provider serves at the same time.
type contaConcorrencia struct {
	agora, maximo atomic.Int64
	h             http.Handler
}

func (c *contaConcorrencia) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := c.agora.Add(1)
	defer c.agora.Add(-1)
	for {
		m := c.maximo.Load()
		if n <= m || c.maximo.CompareAndSwap(m, n) {
			break
		}
	}
	c.h.ServeHTTP(w, r)
}

func TestSemaforoNuncaPassaDoLimite(t *testing.T) {
	a := &contaConcorrencia{h: &provedor.Falso{Latencia: 20 * time.Millisecond}}
	r := novoCenario(t, a, &provedor.Falso{}, 2)
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if _, err := chamar(t, r); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if m := a.maximo.Load(); m > 2 {
		t.Fatalf("max concurrency = %d, limit 2", m)
	}
}

func TestSemaforoCheioDevolveOcupado(t *testing.T) {
	r := novoCenario(t, &provedor.Falso{Latencia: time.Second}, &provedor.Falso{}, 1)
	r.esperaSemaforo = 10 * time.Millisecond
	go func() { _, _ = chamar(t, r) }()
	time.Sleep(50 * time.Millisecond)
	if _, err := chamar(t, r); !errors.Is(err, ErrOcupado) {
		t.Fatalf("error = %v, want ErrOcupado", err)
	}
}

// A provider that hangs on every call is the breaker's classic case: the watchdog's cut
// (a cancellation with its own cause) counts as a failure.
func TestBreakerContaWatchdog(t *testing.T) {
	trava := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body) // without reading the body the server doesn't notice the disconnect
		<-r.Context().Done()
	})
	r := novoCenario(t, trava, &provedor.Falso{}, 10)
	errWatchdog := errors.New("watchdog")
	for range 5 {
		ctx, cancel := context.WithCancelCause(context.Background())
		relogio := time.AfterFunc(20*time.Millisecond, func() { cancel(errWatchdog) })
		_, _, _ = r.Chamar(ctx, 1, "rapido", corpoChat())
		relogio.Stop()
		cancel(nil)
	}
	if e := r.Estado(); e[0].Breaker != "open" {
		t.Fatalf("breaker = %+v, want open after 5 watchdog cuts", e)
	}
}

// contaETrava counts the calls it receives and never answers.
type contaETrava struct{ n atomic.Int64 }

func (c *contaETrava) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.n.Add(1)
	_, _ = io.Copy(io.Discard, r.Body) // without reading the body the server doesn't notice the disconnect
	<-r.Context().Done()
}

func chamarComPrazo(r *Roteador, cliente int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if resp, _, err := r.Chamar(ctx, cliente, "rapido", corpoChat()); err == nil {
		resp.Body.Close()
	}
}

// The breaker is per client and destination: a hung provider opens the breaker of whoever suffered
// the timeouts (an exceeded deadline counts as a failure), but one client can't take the destination
// down for the others by sending requests that exceed the deadline on purpose.
func TestBreakerIsoladoPorCliente(t *testing.T) {
	a := &contaETrava{}
	r := novoCenario(t, a, &provedor.Falso{}, 10)
	for range 5 {
		chamarComPrazo(r, 1)
	}
	antes := a.n.Load()
	chamarComPrazo(r, 1)
	if a.n.Load() != antes {
		t.Fatal("client 1: with its breaker open, the hung destination should be skipped")
	}
	chamarComPrazo(r, 2)
	if a.n.Load() != antes+1 {
		t.Fatal("client 2 should not inherit the breaker opened by client 1")
	}
	if e := r.Estado(); e[0].Destino != "a/modelo-a" || e[0].Breaker != "open" || e[0].Abertos != 1 {
		t.Fatalf("state = %+v", e)
	}
}

// A provider 401/402/403 is a problem with the gateway's account (invalid key, no balance), not with
// the client's request: try the next destination.
func TestContaDoProvedorComProblemaFazFallback(t *testing.T) {
	semSaldo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":{"message":"insufficient balance"}}`))
	})
	b := &provedor.Falso{}
	r := novoCenario(t, semSaldo, b, 10)
	d, err := chamar(t, r)
	if err != nil || d.Provedor != "b" {
		t.Fatalf("destination = %v, error = %v: want fallback b", d, err)
	}
}
