package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/venturelli-91/comporta_proxy/internal/gateway"
	"github.com/venturelli-91/comporta_proxy/internal/lgpd"
)

var errTimeout = errors.New("provedor sem progresso")

// vigia é o watchdog do stream: se passar o prazo sem um "data:" chegar, cancela o contexto
// da chamada com errTimeout. Começa a contar antes do provedor mandar os cabeçalhos, então
// cobre também o provedor que aceita a conexão e trava. Keep-alive (": ...") não conta.
type vigia struct {
	r     io.Reader
	t     *time.Timer
	entre time.Duration
}

func (s *Servidor) novaVigia(cancel context.CancelCauseFunc) *vigia {
	return &vigia{t: time.AfterFunc(s.primeiroToken, func() { cancel(errTimeout) }), entre: s.entreChunks}
}

func (v *vigia) Read(p []byte) (int, error) {
	n, err := v.r.Read(p)
	if bytes.Contains(p[:n], []byte("data:")) {
		v.t.Reset(v.entre)
	}
	return n, err
}

func (v *vigia) parar() { v.t.Stop() }

// causa traduz o erro de leitura no motivo real: prazo do vigia, cliente que saiu ou falha.
func causa(ctx context.Context, err error) error {
	if c := context.Cause(ctx); c != nil {
		return c
	}
	return err
}

// relaySSE lê o stream (ao vivo ou do cache) linha a linha e repassa cada evento com flush,
// para o primeiro token chegar ao cliente sem atraso. No caminho: ignora keep-alive, restaura
// dados pessoais (com buffer para marcador partido), captura o usage e só repassa o chunk de
// usage se o cliente pediu. Os cabeçalhos só saem no primeiro dado útil: até lá ainda dá
// para responder um erro de verdade (504) em vez de um stream vazio.
func (s *Servidor) relaySSE(ctx context.Context, w http.ResponseWriter, corpo io.Reader, p *pedido) (gateway.Uso, error) {
	var st *lgpd.Stream
	if p.mapa.Len() > 0 {
		st = p.mapa.NovoStream()
	}
	rc := http.NewResponseController(w)
	var uso gateway.Uso
	escrever := func(dado []byte) error {
		if p.ev.Status == 0 {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			p.ev.Status, p.ev.TTFTMs = http.StatusOK, time.Since(p.ev.TS).Milliseconds()
		}
		n, err := fmt.Fprintf(w, "data: %s\n\n", dado)
		p.ev.TamanhoResposta += int64(n)
		if err == nil {
			err = rc.Flush()
		}
		if err != nil {
			// Escrita que falha no meio do stream = o cliente foi embora.
			return fmt.Errorf("cliente desconectou: %w", context.Canceled)
		}
		return nil
	}
	// O que ficou retido no restaurador sai num chunk extra antes do [DONE].
	soltarResto := func() error {
		if st == nil {
			return nil
		}
		resto := st.Finalizar()
		if resto == "" {
			return nil
		}
		b, _ := json.Marshal(map[string]any{"object": "chat.completion.chunk",
			"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": resto}}}})
		return escrever(b)
	}

	sc := bufio.NewScanner(corpo)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		dado, eh := bytes.CutPrefix(sc.Bytes(), []byte("data:"))
		if !eh {
			continue // keep-alive, comentário ou separador
		}
		dado = bytes.TrimSpace(dado)
		if bytes.Equal(dado, []byte("[DONE]")) {
			if err := soltarResto(); err != nil {
				return uso, err
			}
			if err := escrever(dado); err != nil {
				return uso, err
			}
			continue
		}
		p.chunks++
		// Só o chunk com usage é decodificado para isso; os demais seguem como bytes.
		if bytes.Contains(dado, []byte(`"usage"`)) {
			var v struct {
				Usage   *gateway.Uso
				Choices []json.RawMessage
			}
			if json.Unmarshal(dado, &v) == nil && v.Usage != nil {
				uso = *v.Usage
				// O gateway pede include_usage para medir custo; o cliente que não pediu não
				// recebe o chunk sem choices (o laço chunk.choices[0] dos SDKs quebraria).
				if len(v.Choices) == 0 && !p.pediuUso {
					continue
				}
			}
		}
		if st != nil {
			dado = restaurarChunk(dado, st)
		}
		if err := escrever(dado); err != nil {
			return uso, err
		}
	}
	if err := sc.Err(); err != nil {
		return uso, causa(ctx, err)
	}
	if ctx.Err() != nil {
		return uso, causa(ctx, ctx.Err())
	}
	return uso, soltarResto()
}

// restaurarChunk troca os marcadores no delta.content de um chunk.
// ponytail: um restaurador para todas as choices; com n > 1 seria um por índice.
func restaurarChunk(dado []byte, st *lgpd.Stream) []byte {
	var v map[string]any
	d := json.NewDecoder(bytes.NewReader(dado))
	d.UseNumber() // não transformar ids e timestamps em float
	if d.Decode(&v) != nil {
		return dado
	}
	choices, _ := v["choices"].([]any)
	for _, c := range choices {
		m, _ := c.(map[string]any)
		delta, _ := m["delta"].(map[string]any)
		if t, ok := delta["content"].(string); ok {
			delta["content"] = st.Escrever(t)
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return dado
	}
	return b
}

// usoDe extrai o usage de uma resposta inteira: o JSON, ou o último chunk do stream que o tiver.
func usoDe(b []byte, stream bool) gateway.Uso {
	var v struct{ Usage *gateway.Uso }
	if !stream {
		if json.Unmarshal(b, &v) == nil && v.Usage != nil {
			return *v.Usage
		}
		return gateway.Uso{}
	}
	var uso gateway.Uso
	for l := range bytes.SplitSeq(b, []byte("\n")) {
		if dado, ok := bytes.CutPrefix(l, []byte("data:")); ok && bytes.Contains(dado, []byte(`"usage"`)) {
			if v.Usage = nil; json.Unmarshal(dado, &v) == nil && v.Usage != nil {
				uso = *v.Usage
			}
		}
	}
	return uso
}

// mascararMensagens troca os dados pessoais de messages[].content (texto) por marcadores.
// Todas as mensagens são mascaradas juntas para um marcador literal de uma não colidir com
// o dado real de outra.
// ponytail: content em partes (multimodal) passa sem máscara; tratar quando houver cliente usando.
func mascararMensagens(corpo map[string]json.RawMessage, mapa *lgpd.Mapa) error {
	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(corpo["messages"], &msgs); err != nil {
		return err
	}
	var textos []string
	var alvos []map[string]json.RawMessage
	for _, m := range msgs {
		var t string
		if json.Unmarshal(m["content"], &t) == nil {
			textos, alvos = append(textos, t), append(alvos, m)
		}
	}
	for i, t := range mapa.MascararVarios(textos) {
		alvos[i]["content"], _ = json.Marshal(t)
	}
	b, err := json.Marshal(msgs)
	corpo["messages"] = b
	return err
}
