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

// relaySSE lê o stream do provedor linha a linha e repassa cada evento com flush, para o
// primeiro token chegar ao cliente sem atraso. No caminho: ignora `: keep-alive`, restaura
// dados pessoais (com buffer para marcador partido), captura o `usage` do último chunk e
// aplica o watchdog (prazo até o primeiro dado e entre dados; keep-alive não conta).
// Os cabeçalhos só são enviados no primeiro dado útil: até lá ainda dá para responder um
// erro de verdade (504) em vez de um stream vazio.
func (s *Servidor) relaySSE(ctx context.Context, w http.ResponseWriter, corpo io.Reader, p *pedido, aoVivo bool) (gateway.Uso, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	linhas := make(chan []byte)
	falha := make(chan error, 1)
	go func() {
		defer close(linhas)
		sc := bufio.NewScanner(corpo)
		sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
		for sc.Scan() {
			select {
			case linhas <- bytes.Clone(sc.Bytes()):
			case <-ctx.Done():
				return
			}
		}
		falha <- sc.Err()
	}()

	limite := s.primeiroToken
	if !aoVivo {
		limite = s.total // replay do cache: os bytes já estão em memória
	}
	timer := time.NewTimer(limite)
	defer timer.Stop()

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

	for {
		select {
		case l, ok := <-linhas:
			if !ok {
				if ctx.Err() != nil {
					return uso, ctx.Err()
				}
				if err := <-falha; err != nil {
					return uso, err
				}
				return uso, soltarResto()
			}
			dado, eh := bytes.CutPrefix(l, []byte("data:"))
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
			// Só o chunk com usage é decodificado só para isso; os demais seguem como bytes.
			if bytes.Contains(dado, []byte(`"usage"`)) {
				var v struct{ Usage *gateway.Uso }
				if json.Unmarshal(dado, &v) == nil && v.Usage != nil {
					uso = *v.Usage
				}
			}
			if st != nil {
				dado = restaurarChunk(dado, st)
			}
			if err := escrever(dado); err != nil {
				return uso, err
			}
			timer.Reset(s.entreChunks)
		case <-timer.C:
			return uso, errTimeout
		case <-ctx.Done():
			return uso, ctx.Err()
		}
	}
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
// ponytail: content em partes (multimodal) passa sem máscara; tratar quando houver cliente usando.
func mascararMensagens(corpo map[string]json.RawMessage, mapa *lgpd.Mapa) error {
	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(corpo["messages"], &msgs); err != nil {
		return err
	}
	for _, m := range msgs {
		var t string
		if json.Unmarshal(m["content"], &t) == nil {
			m["content"], _ = json.Marshal(mapa.Mascarar(t))
		}
	}
	b, err := json.Marshal(msgs)
	corpo["messages"] = b
	return err
}
