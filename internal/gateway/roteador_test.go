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

// novoCenario sobe um servidor HTTP por provedor e um roteador com o alias "rapido" → [a, b].
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
	resp, d, err := r.Chamar(context.Background(), "rapido", corpoChat())
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
		t.Fatalf("destino = %v, queria b/modelo-b", d)
	}
	if a.Chamadas.Load() != 3 {
		t.Errorf("principal recebeu %d chamadas, queria 3 (1 + 2 retries)", a.Chamadas.Load())
	}
}

func TestErroDoClienteNaoFazFallback(t *testing.T) {
	a := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"parâmetro inválido"}}`))
	})
	b := &provedor.Falso{}
	r := novoCenario(t, a, b, 10)
	_, err := chamar(t, r)
	var e *provedor.ErroHTTP
	if !errors.As(err, &e) || e.Status != 400 {
		t.Fatalf("erro = %v, queria 400 do provedor", err)
	}
	if b.Chamadas.Load() != 0 {
		t.Fatal("400 é erro do cliente: não devia tentar o fallback")
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
		t.Fatal("com o breaker aberto, o principal não devia ser chamado")
	}
	if e := r.Estado(); e[0].Destino != "a/modelo-a" || e[0].Breaker != "open" {
		t.Fatalf("estado = %+v", e)
	}
}

func TestSemDestinoQuandoTodosOsBreakersAbrem(t *testing.T) {
	r := novoCenario(t, &provedor.Falso{TaxaErro: 1}, &provedor.Falso{TaxaErro: 1}, 10)
	var err error
	for range 6 {
		_, err = chamar(t, r)
	}
	if !errors.Is(err, ErrSemDestino) {
		t.Fatalf("erro = %v, queria ErrSemDestino", err)
	}
}

// contaConcorrencia mede quantas requisições o provedor atende ao mesmo tempo.
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
		t.Fatalf("concorrência máxima = %d, limite 2", m)
	}
}

func TestSemaforoCheioDevolveOcupado(t *testing.T) {
	r := novoCenario(t, &provedor.Falso{Latencia: time.Second}, &provedor.Falso{}, 1)
	r.esperaSemaforo = 10 * time.Millisecond
	go func() { _, _ = chamar(t, r) }()
	time.Sleep(50 * time.Millisecond)
	if _, err := chamar(t, r); !errors.Is(err, ErrOcupado) {
		t.Fatalf("erro = %v, queria ErrOcupado", err)
	}
}

// Um provedor que trava em toda chamada é o caso clássico do breaker: o prazo estourado
// pelo próprio gateway tem que contar como falha (só o cancelamento do cliente é excluído).
func TestBreakerContaTimeout(t *testing.T) {
	trava := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body) // sem ler o corpo o servidor não percebe a desconexão
		<-r.Context().Done()
	})
	r := novoCenario(t, trava, &provedor.Falso{}, 10)
	for range 5 {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		_, _, _ = r.Chamar(ctx, "rapido", corpoChat())
		cancel()
	}
	if e := r.Estado(); e[0].Breaker != "open" {
		t.Fatalf("breaker = %+v, queria aberto depois de 5 timeouts", e)
	}
}
