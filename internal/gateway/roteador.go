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
	// ErrSemDestino: todos os destinos do alias estão com o breaker aberto (vira 503).
	ErrSemDestino = errors.New("nenhum destino disponível")
	// ErrOcupado: o semáforo do alias ficou cheio além da espera curta (vira 429).
	ErrOcupado = errors.New("modelo ocupado")
)

// Roteador leva um alias ("rapido") até um provedor de verdade, protegendo cada destino com
// circuit breaker e retry, e o alias inteiro com um semáforo abaixo do limite da conta.
//
// O breaker é por cliente e destino. Compartilhado, qualquer falha que um cliente conseguisse
// provocar (gerações que estouram o prazo, pedidos que fazem o provedor travar) abriria o
// destino para todos. Isolado, cada cliente só derruba o próprio caminho, e o breaker pode
// contar timeouts sem medo. O custo: com o provedor doente de verdade, cada cliente precisa
// das próprias falhas para o breaker dele abrir.
type Roteador struct {
	cfg      *Config
	clientes map[string]*provedor.Cliente
	semaforo map[string]chan struct{} // por alias

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

// breaker devolve (criando na primeira vez) o breaker do cliente para o destino.
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
		// Abre com metade das chamadas falhando, mas só depois de uma amostra mínima:
		// uma falha isolada no começo não deve derrubar o destino.
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.Requests >= 5 && c.TotalFailures*2 >= c.Requests
		},
		// Pedido malformado (400, 413, 422) não indica provedor doente.
		IsSuccessful: func(err error) bool { return err == nil || erroDoPedido(err) },
		// Cliente que desistiu não diz nada sobre a saúde do provedor. Prazo estourado e corte
		// do watchdog contam: como o breaker é do próprio cliente, ele só derruba o próprio caminho.
		IsExcluded: func(err error) bool { return errors.Is(err, context.Canceled) },
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
func (r *Roteador) ChamarDestino(ctx context.Context, cliente int64, d Destino, corpo map[string]json.RawMessage) (*http.Response, error) {
	c := maps.Clone(corpo)
	c["model"], _ = json.Marshal(d.Modelo)
	b, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("codificando corpo: %w", err)
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
	Breaker string `json:"breaker"` // pior estado entre os clientes
	Abertos int    `json:"abertos"` // clientes com o breaker não fechado
}

// Estado resume os breakers por destino, em ordem alfabética, para o painel.
// ponytail: sem ocupação do semáforo; entra junto com as métricas Prometheus.
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
