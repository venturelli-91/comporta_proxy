package provedor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const corpoTeste = `{"model":"m","messages":[{"role":"user","content":"olá mundo"}]}`

func novoFalso(t *testing.T, f *Falso) *Cliente {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return Novo(srv.URL, "segredo", 10, time.Second)
}

func TestEnviarSemStream(t *testing.T) {
	c := novoFalso(t, &Falso{})
	resp, err := c.Enviar(context.Background(), []byte(corpoTeste))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.Choices[0].Message.Content, "olá mundo") || v.Usage.PromptTokens == 0 {
		t.Fatalf("resposta inesperada: %+v", v)
	}
}

func TestEnviarStream(t *testing.T) {
	c := novoFalso(t, &Falso{})
	corpo := `{"model":"m","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"oi"}]}`
	resp, err := c.Enviar(context.Background(), []byte(corpo))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	s := string(b)
	for _, quer := range []string{": keep-alive", `"usage"`, "data: [DONE]"} {
		if !strings.Contains(s, quer) {
			t.Errorf("stream sem %q", quer)
		}
	}
}

func TestEnviarErros(t *testing.T) {
	t.Run("503 do falso é repetível", func(t *testing.T) {
		_, err := novoFalso(t, &Falso{TaxaErro: 1}).Enviar(context.Background(), []byte(corpoTeste))
		var e *ErroHTTP
		if !errors.As(err, &e) || e.Status != 503 || !e.Repetivel() {
			t.Fatalf("erro = %v", err)
		}
	})
	t.Run("429 traz Retry-After e mensagem", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"error":{"message":"devagar"}}`))
		}))
		defer srv.Close()
		_, err := Novo(srv.URL, "", 1, time.Second).Enviar(context.Background(), []byte(corpoTeste))
		var e *ErroHTTP
		if !errors.As(err, &e) || e.RetryAfter != 2*time.Second || e.Mensagem != "devagar" {
			t.Fatalf("erro = %#v", err)
		}
	})
	t.Run("400 não é repetível", func(t *testing.T) {
		_, err := novoFalso(t, &Falso{}).Enviar(context.Background(), []byte(`{}`))
		var e *ErroHTTP
		if !errors.As(err, &e) || e.Status != 400 || e.Repetivel() {
			t.Fatalf("erro = %v", err)
		}
	})
	t.Run("rede fora do ar é repetível", func(t *testing.T) {
		_, err := Novo("http://127.0.0.1:1", "", 1, time.Second).Enviar(context.Background(), []byte(corpoTeste))
		var e *ErroHTTP
		if !errors.As(err, &e) || e.Status != 0 || !e.Repetivel() {
			t.Fatalf("erro = %v", err)
		}
	})
	t.Run("contexto cancelado devolve o erro do contexto", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err := novoFalso(t, &Falso{Latencia: 5 * time.Second}).Enviar(ctx, []byte(corpoTeste))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("erro = %v", err)
		}
	})
}
