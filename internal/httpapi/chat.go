package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	maxCorpo    = 1 << 20  // 1 MiB per request
	maxResposta = 16 << 20 // ceiling for responses read whole (non-streaming, cache)
)

var errRespostaGrande = errors.New("provider response above 16 MiB")

// usoEstimado (estimated usage): the provider charged but the usage never arrived (cut stream,
// response above the ceiling). ~4 bytes per input token; the output comes from the caller, so the budget holds.
// ponytail: the event isn't flagged as estimated; a new column when the dashboard needs it.
func usoEstimado(p *pedido, saida int64) gateway.Uso {
	return gateway.Uso{PromptTokens: int64(len(p.corpo["messages"]) / 4), CompletionTokens: saida}
}

// lerResposta reads the whole response up to the ceiling. It reads 1 extra byte to tell "fit" from
// "was cut": cut becomes an error (502), never an incomplete 200 response or a cache entry.
func lerResposta(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxResposta+1))
	if err == nil && len(b) > maxResposta {
		return nil, errRespostaGrande
	}
	return b, err
}

// pedido is the state of a chat request along the path.
type pedido struct {
	cli    armazenamento.Cliente
	alias  string
	corpo  map[string]json.RawMessage
	stream bool
	mapa   *lgpd.Mapa
	ev     armazenamento.Evento

	pediuUso bool // the client asked for stream_options.include_usage
	chunks   int64
}

func (s *Servidor) chat(w http.ResponseWriter, r *http.Request) {
	p := &pedido{cli: clienteDe(r.Context()), mapa: lgpd.NovoMapa()}
	p.ev = armazenamento.Evento{TS: time.Now(), ClienteID: p.cli.ID, RequestID: r.Context().Value(ctxRequestID).(string)}
	defer func() {
		p.ev.LatenciaMs = time.Since(p.ev.TS).Milliseconds()
		s.registrar(p.ev)
	}()

	// Admission before any expensive work.
	if !s.ocupar(p.cli.ID) {
		retryAfter(w, time.Second)
		s.falhar(w, p, http.StatusTooManyRequests, "concorrencia_excedida", "too many concurrent requests for this key")
		return
	}
	defer s.soltar(p.cli.ID)
	if ok, espera := s.permitir(p.cli); !ok {
		retryAfter(w, espera)
		s.falhar(w, p, http.StatusTooManyRequests, "limite_excedido", "requests per minute limit exceeded")
		return
	}
	if s.orcamentoEsgotado(p.cli) {
		s.falhar(w, p, http.StatusPaymentRequired, "orcamento_esgotado", "monthly budget exhausted")
		return
	}

	// Decode into a map of RawMessage: fields the gateway doesn't know go through untouched.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCorpo)).Decode(&p.corpo); err != nil {
		if grande := new(http.MaxBytesError); errors.As(err, &grande) {
			s.falhar(w, p, http.StatusRequestEntityTooLarge, "corpo_grande", "body above 1 MiB")
		} else {
			s.falhar(w, p, http.StatusBadRequest, "corpo_invalido", "invalid JSON")
		}
		return
	}
	_ = json.Unmarshal(p.corpo["model"], &p.alias)
	_ = json.Unmarshal(p.corpo["stream"], &p.stream)
	p.ev.Modelo, p.ev.Stream = p.alias, p.stream
	m, existe := s.d.Config.Modelos[p.alias]
	if !existe {
		s.falhar(w, p, http.StatusBadRequest, "modelo_desconhecido", fmt.Sprintf("model %q does not exist", p.alias))
		return
	}
	if len(p.cli.Modelos) > 0 && !slices.Contains(p.cli.Modelos, p.alias) {
		s.falhar(w, p, http.StatusForbidden, "modelo_nao_liberado", fmt.Sprintf("model %q is not allowed for this key", p.alias))
		return
	}

	// LGPD before the cache: the cache key and everything that leaves from here only see placeholders.
	if p.cli.LGPD {
		if err := mascararMensagens(p.corpo, p.mapa); err != nil {
			s.falhar(w, p, http.StatusBadRequest, "corpo_invalido", "invalid messages")
			return
		}
		p.ev.PII = p.mapa.Len()
		s.met.pii.Add(int64(p.ev.PII))
		// The Playground shows what the provider received next to what came back. Safe to return:
		// the text is already masked and it's the client's own message. Base64 because headers only
		// take ASCII; cut at 2 KB because proxies in front limit header size.
		if t := ultimaDoUsuario(p.corpo); t != "" {
			w.Header().Set("X-Comporta-Enviado", base64.StdEncoding.EncodeToString([]byte(t)))
		}
	}
	if p.stream {
		// Without include_usage the stream carries no usage and the cost would be unknown. The client's
		// other stream_options fields are kept.
		opcoes := map[string]any{}
		_ = json.Unmarshal(p.corpo["stream_options"], &opcoes)
		p.pediuUso = opcoes["include_usage"] == true
		opcoes["include_usage"] = true
		p.corpo["stream_options"], _ = json.Marshal(opcoes)
	}
	if p.cli.Cache || r.Header.Get("X-Comporta-Cache") == "ligado" {
		s.comCache(w, r, p)
		return
	}
	// Shadow only on the uncached path: comparing a reused response measures nothing and pays for a call for nothing.
	if m.Shadow != nil {
		s.sombra(p, *m.Shadow)
	}
	s.direto(w, r, p)
}

// direto (direct): no cache. With streaming, relays live; the request's context goes all the way
// to the provider, so a client that disconnects cancels the paid call.
func (s *Servidor) direto(w http.ResponseWriter, r *http.Request, p *pedido) {
	ctx, cancel := context.WithCancelCause(r.Context())
	defer cancel(nil)
	var v *vigia
	if p.stream {
		v = s.novaVigia(cancel)
		defer v.parar()
	} else {
		var fim context.CancelFunc
		ctx, fim = context.WithTimeout(ctx, s.total)
		defer fim()
	}
	resp, dest, err := s.d.Roteador.Chamar(ctx, p.cli.ID, p.alias, p.corpo)
	if err != nil {
		s.erroProvedor(w, p, causa(ctx, err))
		return
	}
	defer resp.Body.Close()
	p.ev.Destino = dest.String()

	var uso gateway.Uso
	if p.stream {
		v.r = resp.Body
		uso, err = s.relaySSE(ctx, w, v, p)
		if uso == (gateway.Uso{}) {
			// Stream cut before the usage chunk: the provider charges anyway (~1 token per chunk).
			uso = usoEstimado(p, p.chunks)
		}
	} else {
		var b []byte
		if b, err = lerResposta(resp.Body); err == nil {
			uso = usoDe(b, false)
			s.responderCorpo(w, p, b)
		} else if errors.Is(err, errRespostaGrande) {
			uso = usoEstimado(p, maxResposta/4) // the provider charged for the whole response
		}
	}
	if err != nil {
		s.erroRelay(w, p, err)
	}
	s.cobrar(p, dest, uso)
}

// resultadoSF is what the singleflight leader hands to everyone waiting on the same key.
type resultadoSF struct {
	e    cache.Entrada
	dest gateway.Destino
	uso  gateway.Uso
}

// comCache (with cache): a hit answers from memory. A miss goes through the singleflight: N identical
// concurrent requests become 1 call; the leader reads the whole response, stores it in the cache and
// everyone gets the same bytes (as a replay when streaming).
// ponytail: followers don't get the live stream; fan-out waits until the TTFT of cached requests matters.
func (s *Servidor) comCache(w http.ResponseWriter, r *http.Request, p *pedido) {
	b, _ := json.Marshal(p.corpo) // json.Marshal sorts map keys: same body → same key
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
		// The leader must not cancel everyone's call if it disconnects.
		base, fim := context.WithTimeout(context.WithoutCancel(r.Context()), s.total)
		defer fim()
		ctx, cancel := context.WithCancelCause(base)
		defer cancel(nil)
		var v *vigia
		if p.stream {
			v = s.novaVigia(cancel) // the same watchdog as the direct path
			defer v.parar()
		}
		resp, dest, err := s.d.Roteador.Chamar(ctx, p.cli.ID, p.alias, p.corpo)
		if err != nil {
			return nil, causa(ctx, err)
		}
		defer resp.Body.Close()
		var leitor io.Reader = resp.Body
		if v != nil {
			v.r, leitor = resp.Body, v
		}
		corpo, err := lerResposta(leitor)
		if err != nil {
			if errors.Is(err, errRespostaGrande) {
				s.cobrar(p, dest, usoEstimado(p, maxResposta/4)) // the leader pays what the provider charged
			}
			return nil, causa(ctx, err)
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
	if _, err := s.relaySSE(r.Context(), w, bytes.NewReader(e.Corpo), p); err != nil {
		s.erroRelay(w, p, err)
	}
}

// responderCorpo returns a non-streaming JSON with the personal data restored. Restored values
// (digits, email, phone) have no quotes or backslashes, so replacing them directly in the JSON
// doesn't break the format.
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

// sombra (shadow) sends the same request (non-streaming) to the shadow destination in the background.
// The client never waits for or receives that response; only the comparison is recorded.
func (s *Servidor) sombra(p *pedido, d gateway.Destino) {
	corpo := maps.Clone(p.corpo)
	corpo["stream"] = json.RawMessage("false")
	delete(corpo, "stream_options")
	ev := armazenamento.Evento{TS: time.Now(), ClienteID: p.cli.ID, RequestID: p.ev.RequestID,
		Modelo: p.alias, Destino: d.String(), Shadow: true, PII: p.ev.PII}
	select {
	case s.sombras <- struct{}{}:
	default:
		s.met.sombrasDescartadas.Add(1)
		return
	}
	s.bg.Go(func() {
		defer func() { <-s.sombras }()
		ctx, cancel := context.WithTimeout(s.ctxSombras, s.total)
		defer cancel()
		resp, err := s.d.Roteador.ChamarDestino(ctx, p.cli.ID, d, corpo)
		if err != nil {
			ev.Status = statusDe(err)
		} else {
			b, err := lerResposta(resp.Body)
			resp.Body.Close()
			uso := usoDe(b, false)
			ev.Status, ev.TamanhoResposta = http.StatusOK, int64(len(b))
			if err != nil {
				ev.Status = statusDe(err)
			}
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

// statusDe translates an error on the way to the provider into the status returned to the client.
func statusDe(err error) int {
	var e *provedor.ErroHTTP
	switch {
	case errors.Is(err, context.Canceled):
		return 499 // nginx convention: the client gave up
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, errTimeout):
		return http.StatusGatewayTimeout
	case errors.Is(err, gateway.ErrOcupado):
		return http.StatusTooManyRequests
	case errors.Is(err, gateway.ErrSemDestino):
		return http.StatusServiceUnavailable
	case errors.As(err, &e) && e.Status == 429:
		return http.StatusTooManyRequests
	case errors.As(err, &e) && (e.Status == 400 || e.Status == 422):
		return e.Status // malformed request: the client needs to know
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
		erro(w, st, "provedor_ocupado", "provider overloaded, try again")
	case http.StatusServiceUnavailable:
		erro(w, st, "sem_destino", "no provider available right now")
	case http.StatusGatewayTimeout:
		erro(w, st, "timeout", "the provider did not respond in time")
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		errors.As(err, &e)
		erro(w, st, "pedido_invalido", e.Mensagem)
	default:
		s.d.Log.Warn("provider", "request_id", p.ev.RequestID, "error", err.Error())
		erro(w, st, "erro_provedor", "the provider failed")
	}
}

// erroRelay handles failures after the provider's response has started: if nothing has been
// written yet, a real error can still be sent; otherwise all that's left is to record and finish.
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
