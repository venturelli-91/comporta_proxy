package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/venturelli-91/comporta_proxy/internal/armazenamento"
	"github.com/venturelli-91/comporta_proxy/internal/cache"
	"github.com/venturelli-91/comporta_proxy/internal/gateway"
	"github.com/venturelli-91/comporta_proxy/internal/lgpd"
	"github.com/venturelli-91/comporta_proxy/internal/provedor"
)

const (
	maxCorpo    = 1 << 20  // 1 MiB por requisição
	maxResposta = 16 << 20 // teto para respostas lidas inteiras (sem stream, cache)
)

// pedido é o estado de uma requisição de chat ao longo do caminho.
type pedido struct {
	cli    armazenamento.Cliente
	alias  string
	corpo  map[string]json.RawMessage
	stream bool
	mapa   *lgpd.Mapa
	ev     armazenamento.Evento
}

func (s *Servidor) chat(w http.ResponseWriter, r *http.Request) {
	p := &pedido{cli: clienteDe(r.Context()), mapa: lgpd.NovoMapa()}
	p.ev = armazenamento.Evento{TS: time.Now(), ClienteID: p.cli.ID, RequestID: r.Context().Value(ctxRequestID).(string)}
	defer func() {
		p.ev.LatenciaMs = time.Since(p.ev.TS).Milliseconds()
		s.registrar(p.ev)
	}()

	// Admissão antes de qualquer trabalho caro.
	if ok, espera := s.permitir(p.cli); !ok {
		retryAfter(w, espera)
		s.falhar(w, p, http.StatusTooManyRequests, "limite_excedido", "limite de requisições por minuto excedido")
		return
	}
	if s.orcamentoEsgotado(p.cli) {
		s.falhar(w, p, http.StatusPaymentRequired, "orcamento_esgotado", "orçamento mensal esgotado")
		return
	}

	// Decodifica num mapa de RawMessage: campos que o gateway não conhece seguem intactos.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCorpo)).Decode(&p.corpo); err != nil {
		if grande := new(http.MaxBytesError); errors.As(err, &grande) {
			s.falhar(w, p, http.StatusRequestEntityTooLarge, "corpo_grande", "corpo acima de 1 MiB")
		} else {
			s.falhar(w, p, http.StatusBadRequest, "corpo_invalido", "JSON inválido")
		}
		return
	}
	_ = json.Unmarshal(p.corpo["model"], &p.alias)
	_ = json.Unmarshal(p.corpo["stream"], &p.stream)
	p.ev.Modelo, p.ev.Stream = p.alias, p.stream
	m, existe := s.d.Config.Modelos[p.alias]
	if !existe {
		s.falhar(w, p, http.StatusBadRequest, "modelo_desconhecido", fmt.Sprintf("modelo %q não existe", p.alias))
		return
	}
	if len(p.cli.Modelos) > 0 && !slices.Contains(p.cli.Modelos, p.alias) {
		s.falhar(w, p, http.StatusForbidden, "modelo_nao_liberado", fmt.Sprintf("modelo %q não liberado para esta chave", p.alias))
		return
	}

	// LGPD antes do cache: a chave de cache e tudo que sai daqui só veem marcadores.
	if p.cli.LGPD {
		if err := mascararMensagens(p.corpo, p.mapa); err != nil {
			s.falhar(w, p, http.StatusBadRequest, "corpo_invalido", "messages inválido")
			return
		}
		p.ev.PII = p.mapa.Len()
		s.met.pii.Add(int64(p.ev.PII))
	}
	if p.stream {
		// Sem isso o stream não traz usage e o custo da chamada ficaria desconhecido.
		p.corpo["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	}
	if p.cli.Cache || r.Header.Get("X-Comporta-Cache") == "ligado" {
		s.comCache(w, r, p)
		return
	}
	// Shadow só no caminho sem cache: comparar uma resposta reaproveitada não mede nada e paga uma chamada à toa.
	if m.Shadow != nil {
		s.sombra(r.Context(), p, *m.Shadow)
	}
	s.direto(w, r, p)
}

// direto: sem cache. Com stream, repassa ao vivo; o contexto da requisição vai até o
// provedor, então um cliente que desconecta cancela a chamada paga.
func (s *Servidor) direto(w http.ResponseWriter, r *http.Request, p *pedido) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if !p.stream {
		ctx, cancel = context.WithTimeout(ctx, s.total)
		defer cancel()
	}
	resp, dest, err := s.d.Roteador.Chamar(ctx, p.alias, p.corpo)
	if err != nil {
		s.erroProvedor(w, p, err)
		return
	}
	defer resp.Body.Close()
	p.ev.Destino = dest.String()

	var uso gateway.Uso
	if p.stream {
		uso, err = s.relaySSE(ctx, w, resp.Body, p, true)
	} else {
		var b []byte
		if b, err = io.ReadAll(io.LimitReader(resp.Body, maxResposta)); err == nil {
			uso = usoDe(b, false)
			s.responderCorpo(w, p, b)
		}
	}
	if err != nil {
		s.erroRelay(w, p, err)
	}
	s.cobrar(p, dest, uso)
}

// resultadoSF é o que o líder do singleflight entrega a todos que esperavam a mesma chave.
type resultadoSF struct {
	e    cache.Entrada
	dest gateway.Destino
	uso  gateway.Uso
}

// comCache: hit responde da memória. Miss passa pelo singleflight: N pedidos iguais ao mesmo
// tempo viram 1 chamada; o líder lê a resposta inteira, grava no cache e todos recebem os
// mesmos bytes (em stream, como replay).
// ponytail: os seguidores não recebem o stream ao vivo; fanout fica para quando o TTFT de
// requisições cacheadas importar.
func (s *Servidor) comCache(w http.ResponseWriter, r *http.Request, p *pedido) {
	b, _ := json.Marshal(p.corpo) // json.Marshal ordena as chaves do mapa: corpo igual → chave igual
	h := sha256.Sum256(append([]byte(strconv.FormatInt(p.cli.ID, 10)+"\x00"), b...))
	chave := hex.EncodeToString(h[:])

	if e, ok := s.d.Cache.Get(chave); ok {
		s.met.cacheHits.Add(1)
		p.ev.Cache, p.ev.EconomiaMicro = true, e.Custo
		s.entregar(w, r, p, e)
		return
	}
	lider := false
	v, err, _ := s.sf.Do(chave, func() (any, error) {
		lider = true
		// O líder não pode cancelar a chamada de todos se desconectar.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), s.total)
		defer cancel()
		resp, dest, err := s.d.Roteador.Chamar(ctx, p.alias, p.corpo)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		corpo, err := io.ReadAll(io.LimitReader(resp.Body, maxResposta))
		if err != nil {
			return nil, err
		}
		uso := usoDe(corpo, p.stream)
		e := cache.Entrada{Corpo: corpo, Stream: p.stream, Custo: s.d.Config.Custo(dest.Modelo, uso, time.Now())}
		s.d.Cache.Set(chave, e)
		return resultadoSF{e, dest, uso}, nil
	})
	if err != nil {
		s.erroProvedor(w, p, err)
		return
	}
	res := v.(resultadoSF)
	if lider {
		s.cobrar(p, res.dest, res.uso)
	} else {
		s.met.cacheHits.Add(1)
		p.ev.Cache, p.ev.EconomiaMicro = true, res.e.Custo
	}
	s.entregar(w, r, p, res.e)
}

func (s *Servidor) entregar(w http.ResponseWriter, r *http.Request, p *pedido, e cache.Entrada) {
	if !e.Stream {
		s.responderCorpo(w, p, e.Corpo)
		return
	}
	if _, err := s.relaySSE(r.Context(), w, bytes.NewReader(e.Corpo), p, false); err != nil {
		s.erroRelay(w, p, err)
	}
}

// responderCorpo devolve um JSON sem stream, com os dados pessoais restaurados. Os valores
// restaurados (dígitos, e-mail, telefone) não têm aspas nem barras, então trocar direto no
// JSON não quebra o formato.
func (s *Servidor) responderCorpo(w http.ResponseWriter, p *pedido, b []byte) {
	if p.mapa.Len() > 0 {
		b = []byte(p.mapa.Restaurar(string(b)))
	}
	p.ev.Status, p.ev.TamanhoResposta = http.StatusOK, int64(len(b))
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func (s *Servidor) cobrar(p *pedido, dest gateway.Destino, uso gateway.Uso) {
	p.ev.Destino = dest.String()
	p.ev.PromptTokens, p.ev.CompletionTokens = uso.PromptTokens, uso.CompletionTokens
	p.ev.CacheHitTokens, p.ev.ReasoningTokens = uso.CacheHitTokens, uso.Detalhes.ReasoningTokens
	p.ev.CustoMicro = s.d.Config.Custo(dest.Modelo, uso, time.Now())
	s.gastar(p.cli.ID, p.ev.CustoMicro)
}

// sombra manda a mesma requisição (sem stream) ao destino shadow em segundo plano. O cliente
// nunca espera nem recebe essa resposta; só a comparação fica registrada.
func (s *Servidor) sombra(ctx context.Context, p *pedido, d gateway.Destino) {
	corpo := maps.Clone(p.corpo)
	corpo["stream"] = json.RawMessage("false")
	delete(corpo, "stream_options")
	ev := armazenamento.Evento{TS: time.Now(), ClienteID: p.cli.ID, RequestID: p.ev.RequestID,
		Modelo: p.alias, Destino: d.String(), Shadow: true, PII: p.ev.PII}
	s.bg.Go(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.total)
		defer cancel()
		resp, err := s.d.Roteador.ChamarDestino(ctx, d, corpo)
		if err != nil {
			ev.Status = statusDe(err)
		} else {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, maxResposta))
			resp.Body.Close()
			uso := usoDe(b, false)
			ev.Status, ev.TamanhoResposta = http.StatusOK, int64(len(b))
			ev.PromptTokens, ev.CompletionTokens = uso.PromptTokens, uso.CompletionTokens
			ev.CustoMicro = s.d.Config.Custo(d.Modelo, uso, time.Now())
		}
		ev.LatenciaMs = time.Since(ev.TS).Milliseconds()
		s.registrar(ev)
	})
}

func (s *Servidor) registrar(ev armazenamento.Evento) {
	if !ev.Shadow {
		s.met.requisicoes.Add(1)
		if ev.Status >= 400 {
			s.met.erros.Add(1)
		}
	}
	s.d.Gravador.Registrar(ev)
}

func (s *Servidor) falhar(w http.ResponseWriter, p *pedido, status int, codigo, msg string) {
	p.ev.Status = status
	erro(w, status, codigo, msg)
}

// statusDe traduz o erro do caminho até o provedor para o status devolvido ao cliente.
func statusDe(err error) int {
	var e *provedor.ErroHTTP
	switch {
	case errors.Is(err, context.Canceled):
		return 499 // convenção do nginx: o cliente desistiu
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, errTimeout):
		return http.StatusGatewayTimeout
	case errors.Is(err, gateway.ErrOcupado):
		return http.StatusTooManyRequests
	case errors.Is(err, gateway.ErrSemDestino):
		return http.StatusServiceUnavailable
	case errors.As(err, &e) && e.Status == 429:
		return http.StatusTooManyRequests
	case errors.As(err, &e) && (e.Status == 400 || e.Status == 422):
		return e.Status // pedido malformado: o cliente precisa saber
	default:
		return http.StatusBadGateway
	}
}

func (s *Servidor) erroProvedor(w http.ResponseWriter, p *pedido, err error) {
	st := statusDe(err)
	p.ev.Status = st
	var e *provedor.ErroHTTP
	switch st {
	case 499:
	case http.StatusTooManyRequests:
		retryAfter(w, time.Second)
		erro(w, st, "provedor_ocupado", "provedor sobrecarregado, tente de novo")
	case http.StatusServiceUnavailable:
		erro(w, st, "sem_destino", "nenhum provedor disponível no momento")
	case http.StatusGatewayTimeout:
		erro(w, st, "timeout", "o provedor não respondeu a tempo")
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		errors.As(err, &e)
		erro(w, st, "pedido_invalido", e.Mensagem)
	default:
		s.d.Log.Warn("provedor", "request_id", p.ev.RequestID, "erro", err.Error())
		erro(w, st, "erro_provedor", "o provedor falhou")
	}
}

// erroRelay trata falhas depois que a resposta do provedor começou: se nada foi escrito
// ainda, dá para mandar um erro de verdade; senão só resta registrar e encerrar.
func (s *Servidor) erroRelay(w http.ResponseWriter, p *pedido, err error) {
	if p.ev.Status == 0 {
		s.erroProvedor(w, p, err)
		return
	}
	p.ev.Status = statusDe(err)
}

func (s *Servidor) modelos(w http.ResponseWriter, r *http.Request) {
	cli := clienteDe(r.Context())
	dados := []map[string]string{}
	for _, alias := range slices.Sorted(maps.Keys(s.d.Config.Modelos)) {
		if len(cli.Modelos) == 0 || slices.Contains(cli.Modelos, alias) {
			dados = append(dados, map[string]string{"id": alias, "object": "model", "owned_by": "comporta"})
		}
	}
	responderJSON(w, http.StatusOK, map[string]any{"object": "list", "data": dados})
}
