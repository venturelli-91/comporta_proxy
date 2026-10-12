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
	"strings"
	"time"

	"github.com/venturelli-91/comporta_proxy/internal/gateway"
	"github.com/venturelli-91/comporta_proxy/internal/lgpd"
)

var errTimeout = errors.New("provider made no progress")

// vigia is the stream's watchdog: if the deadline passes without a "data:" arriving, it cancels the
// call's context with errTimeout. It starts counting before the provider sends headers, so it also
// covers a provider that accepts the connection and hangs. Keep-alives (": ...") don't count.
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

// causa translates a read error into the real reason: the watchdog's deadline, a client that left, or a failure.
func causa(ctx context.Context, err error) error {
	if c := context.Cause(ctx); c != nil {
		return c
	}
	return err
}

// relaySSE reads the stream (live or from the cache) line by line and relays each event with a flush,
// so the first token reaches the client without delay. Along the way: ignores keep-alives, restores
// personal data (with a buffer for split placeholders), captures the usage and only relays the usage
// chunk if the client asked for it. Headers only go out on the first useful data: until then a real
// error (504) can still be returned instead of an empty stream.
func (s *Servidor) relaySSE(ctx context.Context, w http.ResponseWriter, corpo io.Reader, p *pedido) (gateway.Uso, error) {
	// One restorer per delta field: content and reasoning_content (DeepSeek's reasoning) are
	// independent texts, and a placeholder can arrive split in either one.
	var sts map[string]*lgpd.Stream
	if p.mapa.Len() > 0 {
		sts = map[string]*lgpd.Stream{"content": p.mapa.NovoStream(), "reasoning_content": p.mapa.NovoStream()}
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
			// A write failing mid-stream = the client went away.
			return fmt.Errorf("client disconnected: %w", context.Canceled)
		}
		return nil
	}
	// Whatever the restorer held back goes out in an extra chunk before [DONE].
	soltarResto := func() error {
		delta := map[string]string{}
		for campo, st := range sts {
			if resto := st.Finalizar(); resto != "" {
				delta[campo] = resto
			}
		}
		if len(delta) == 0 {
			return nil
		}
		b, _ := json.Marshal(map[string]any{"object": "chat.completion.chunk",
			"choices": []any{map[string]any{"index": 0, "delta": delta}}})
		return escrever(b)
	}

	sc := bufio.NewScanner(corpo)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		dado, eh := bytes.CutPrefix(sc.Bytes(), []byte("data:"))
		if !eh {
			continue // keep-alive, comment or separator
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
		// Only the chunk with usage is decoded for it; the others pass through as bytes.
		if bytes.Contains(dado, []byte(`"usage"`)) {
			var v struct {
				Usage   *gateway.Uso
				Choices []json.RawMessage
			}
			if json.Unmarshal(dado, &v) == nil && v.Usage != nil {
				uso = *v.Usage
				// The gateway asks for include_usage to measure cost; a client that didn't ask doesn't
				// get the chunk without choices (the SDKs' chunk.choices[0] loop would break).
				if len(v.Choices) == 0 && !p.pediuUso {
					continue
				}
			}
		}
		if sts != nil {
			dado = restaurarChunk(dado, sts)
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

// restaurarChunk replaces the placeholders in the text fields of a chunk's delta.
// ponytail: one restorer for all choices; with n > 1 it would be one per index.
func restaurarChunk(dado []byte, sts map[string]*lgpd.Stream) []byte {
	var v map[string]any
	d := json.NewDecoder(bytes.NewReader(dado))
	d.UseNumber() // don't turn ids and timestamps into floats
	if d.Decode(&v) != nil {
		return dado
	}
	choices, _ := v["choices"].([]any)
	for _, c := range choices {
		m, _ := c.(map[string]any)
		delta, _ := m["delta"].(map[string]any)
		for campo, st := range sts {
			if t, ok := delta[campo].(string); ok {
				delta[campo] = st.Escrever(t)
			}
		}
		// The answer has started: the reasoning is over. Whatever the restorer held back (a "[" without
		// "]") goes out in this chunk, before the content, instead of in the extra chunk at the end.
		if _, ok := delta["content"].(string); ok {
			if resto := sts["reasoning_content"].Finalizar(); resto != "" {
				r, _ := delta["reasoning_content"].(string)
				delta["reasoning_content"] = r + resto
			}
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return dado
	}
	return b
}

// usoDe extracts the usage from a whole response: the JSON, or the last stream chunk that has it.
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

// errNaoTexto: a non-text content part (image, audio) can't be masked, so with LGPD on the request
// is refused rather than sent with personal data in it (fail closed).
var errNaoTexto = errors.New("non-text content")

// mascararMensagens replaces the personal data in messages[].content with placeholders, whether
// content is a string or an array of parts (as many SDKs send it). All texts are masked together
// so a literal placeholder in one can't collide with real data in another.
// ponytail: tools, tool_calls arguments, name and user still go out unmasked (backlog).
func mascararMensagens(corpo map[string]json.RawMessage, mapa *lgpd.Mapa) error {
	var msgs []map[string]json.RawMessage
	if err := json.Unmarshal(corpo["messages"], &msgs); err != nil {
		return err
	}
	var textos []string
	var gravar []func(string) // writes each masked text back where it came from
	var comPartes []func() error
	for _, m := range msgs {
		bruto := bytes.TrimSpace(m["content"])
		switch {
		case len(bruto) > 0 && bruto[0] == '"':
			var t string
			if err := json.Unmarshal(bruto, &t); err != nil {
				return err
			}
			textos = append(textos, t)
			gravar = append(gravar, func(s string) { m["content"], _ = json.Marshal(s) })
		case len(bruto) > 0 && bruto[0] == '[':
			var partes []map[string]json.RawMessage
			if err := json.Unmarshal(bruto, &partes); err != nil {
				return err
			}
			for _, p := range partes {
				var tipo, t string
				_ = json.Unmarshal(p["type"], &tipo)
				if tipo != "text" {
					return errNaoTexto
				}
				if err := json.Unmarshal(p["text"], &t); err != nil {
					return err
				}
				textos = append(textos, t)
				gravar = append(gravar, func(s string) { p["text"], _ = json.Marshal(s) })
			}
			comPartes = append(comPartes, func() (err error) { m["content"], err = json.Marshal(partes); return err })
		}
		// null or absent content (assistant tool call) has nothing to mask and stays as it is.
	}
	for i, t := range mapa.MascararVarios(textos) {
		gravar[i](t)
	}
	for _, f := range comPartes {
		if err := f(); err != nil {
			return err
		}
	}
	b, err := json.Marshal(msgs)
	corpo["messages"] = b
	return err
}

// ultimaDoUsuario returns the text of the last "user" message (up to 2 KB, without splitting a character).
func ultimaDoUsuario(corpo map[string]json.RawMessage) string {
	var msgs []struct{ Role, Content string }
	_ = json.Unmarshal(corpo["messages"], &msgs) // multipart (multimodal) content stays empty
	for i := len(msgs) - 1; i >= 0; i-- {
		if t := msgs[i].Content; msgs[i].Role == "user" && t != "" {
			if len(t) > 2048 {
				t = strings.ToValidUTF8(t[:2048], "")
			}
			return t
		}
	}
	return ""
}
