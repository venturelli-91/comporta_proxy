package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/venturelli-91/comporta_proxy/internal/provedor"
	"github.com/venturelli-91/comporta_proxy/internal/resiliencia"
)

var (
	// ErrSemDestino: todos os destinos do alias estão com o breaker aberto (vira 503).
	ErrSemDestino = errors.New("nenhum destino disponível")
	// ErrOcupado: o semáforo do alias ficou cheio além da espera curta (vira 429).
	ErrOcupado = errors.New("modelo ocupado")
)

// Roteador leva um alias ("rapido") até um provedor de verdade, protegendo cada destino com
// circuit breaker e retry, e o alias inteiro com um semáforo abaixo do limite da conta.
type Roteador struct {
	cfg      *Config
	clientes map[string]*provedor.Cliente
	breakers map[string]*gobreaker.CircuitBreaker[*http.Response] // por destino "provedor/modelo"
	semaforo map[string]chan struct{}                             // por alias

	baseRetry      time.Duration
	esperaSemaforo time.Duration
}

func NovoRoteador(cfg *Config, clientes map[string]*provedor.Cliente) *Roteador {
	r := &Roteador{
		cfg: cfg, clientes: clientes,
		breakers:       map[string]*gobreaker.CircuitBreaker[*http.Response]{},
		semaforo:       map[string]chan struct{}{},
		baseRetry:      200 * time.Millisecond,
		esperaSemaforo: 2 * time.Second,
	}
	for alias, m := range cfg.Modelos {
		r.semaforo[alias] = make(chan struct{}, max(m.Concorrencia, 1))
		for _, d := range m.todos() {
			if _, ok := r.breakers[d.String()]; !ok {
				r.breakers[d.String()] = novoBreaker(d.String())
			}
		}
	}
	return r
}

func novoBreaker(nome string) *gobreaker.CircuitBreaker[*http.Response] {
	return gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
		Name:        nome,
		MaxRequests: 1,
		Interval:    time.Minute,
		Timeout:     10 * time.Second,
		// Abre com metade das chamadas falhando, mas só depois de uma amostra mínima:
		// uma falha isolada no começo não deve derrubar o destino.
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.Requests >= 5 && c.TotalFailures*2 >= c.Requests
		},
		// Pedido malformado (400, 413, 422) não indica provedor doente.
		IsSuccessful: func(err error) bool { return err == nil || erroDoPedido(err) },
		// O breaker é compartilhado por todos os clientes, então só conta o que é culpa do
		// provedor. Fica de fora o cliente que desistiu e o prazo total do pedido: um cliente
		// mandando gerações enormes de propósito não pode derrubar o destino para os outros.
		// O corte do watchdog (cancelamento com causa própria: provedor sem resposta) conta.
		IsExcluded: func(err error) bool {
			return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
		},
	})
}

// erroDoPedido: o provedor recusou o pedido em si; falharia igual em qualquer destino.
// 401/402/403 ficam de fora de propósito: são problema da conta do gateway no provedor
// (chave inválida, sem saldo) e o próximo destino pode atender.
func erroDoPedido(err error) bool {
	var e *provedor.ErroHTTP
	return errors.As(err, &e) && (e.Status == 400 || e.Status == 413 || e.Status == 422)
}

// Chamar tenta os destinos do alias em ordem. Devolve a resposta 2xx aberta: quem chama lê o
// corpo (stream ou não) e fecha; fechar o corpo também libera o semáforo.
func (r *Roteador) Chamar(ctx context.Context, alias string, corpo map[string]json.RawMessage) (*http.Response, Destino, error) {
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
		resp, err := r.ChamarDestino(ctx, d, corpo)
		if err == nil {
			resp.Body = &corpoComFim{ReadCloser: resp.Body, fim: liberar}
			return resp, d, nil
		}
		ultimo = err
		// Pedido malformado falharia igual no próximo destino; contexto morto não tem para quem responder.
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

// ChamarDestino chama um destino específico (usado também pelo modo shadow), com o retry
// dentro do breaker: o breaker conta só o resultado final de cada chamada.
func (r *Roteador) ChamarDestino(ctx context.Context, d Destino, corpo map[string]json.RawMessage) (*http.Response, error) {
	c := maps.Clone(corpo)
	c["model"], _ = json.Marshal(d.Modelo)
	b, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("codificando corpo: %w", err)
	}
	cliente := r.clientes[d.Provedor]
	return r.breakers[d.String()].Execute(func() (*http.Response, error) {
		var resp *http.Response
		err := resiliencia.Retry(ctx, 3, r.baseRetry, func() (err error) {
			resp, err = cliente.Enviar(ctx, b)
			return err
		})
		return resp, err
	})
}

type EstadoDestino struct {
	Destino string `json:"destino"`
	Breaker string `json:"breaker"`
}

// Estado lista os breakers em ordem alfabética, para o painel.
// ponytail: sem ocupação do semáforo; entra junto com as métricas Prometheus.
func (r *Roteador) Estado() []EstadoDestino {
	out := make([]EstadoDestino, 0, len(r.breakers))
	for nome, b := range r.breakers {
		out = append(out, EstadoDestino{nome, b.State().String()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Destino < out[j].Destino })
	return out
}

// corpoComFim roda `fim` uma vez quando o corpo é fechado.
type corpoComFim struct {
	io.ReadCloser
	fim func()
}

func (c *corpoComFim) Close() error {
	err := c.ReadCloser.Close()
	c.fim()
	return err
}
