// Package provedor fala com qualquer API compatível com OpenAI (a DeepSeek é uma delas).
// Por isso existe um único Cliente: "deepseek" e "openaicompat" diferem só na base_url.
package provedor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

type Cliente struct {
	baseURL string
	chave   string
	http    *http.Client
}

// Novo cria um cliente com transporte próprio. O padrão do Go guarda só 2 conexões ociosas
// por host: sob carga isso força handshakes TLS novos o tempo todo.
func Novo(baseURL, chave string, maxConexoes int, timeoutConexao time.Duration) *Cliente {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = maxConexoes
	t.MaxIdleConnsPerHost = maxConexoes
	t.MaxConnsPerHost = maxConexoes
	t.IdleConnTimeout = 90 * time.Second
	t.ForceAttemptHTTP2 = true
	t.DialContext = (&net.Dialer{Timeout: timeoutConexao, KeepAlive: 30 * time.Second}).DialContext
	// Sem http.Client.Timeout: ele cortaria streams longos. Os prazos vêm do context.
	return &Cliente{baseURL: baseURL, chave: chave, http: &http.Client{Transport: t}}
}

// ErroHTTP é uma falha do provedor. Status 0 significa erro de rede (sem resposta).
type ErroHTTP struct {
	Status     int
	RetryAfter time.Duration
	Mensagem   string
	causa      error
}

func (e *ErroHTTP) Error() string {
	if e.Status == 0 {
		return "provedor inacessível: " + e.Mensagem
	}
	return fmt.Sprintf("provedor respondeu %d: %s", e.Status, e.Mensagem)
}

func (e *ErroHTTP) Unwrap() error { return e.causa }

// Repetivel: só vale tentar de novo quando o problema é do provedor (sobrecarga, 5xx, rede).
// 400/401/402/422 vão falhar igual na próxima tentativa.
func (e *ErroHTTP) Repetivel() bool { return e.Status == 0 || e.Status == 429 || e.Status >= 500 }

func (e *ErroHTTP) Espera() time.Duration { return e.RetryAfter }

// Enviar faz o POST e devolve a resposta só quando ela é 2xx; quem chama fecha o Body.
func (c *Cliente) Enviar(ctx context.Context, corpo []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(corpo))
	if err != nil {
		return nil, fmt.Errorf("montando requisição: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.chave != "" {
		req.Header.Set("Authorization", "Bearer "+c.chave)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			// A causa diz quem cortou: o cliente (Canceled) ou um prazo do gateway.
			return nil, context.Cause(ctx)
		}
		return nil, &ErroHTTP{Mensagem: err.Error(), causa: err}
	}
	if resp.StatusCode/100 == 2 {
		return resp, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	e := &ErroHTTP{Status: resp.StatusCode, Mensagem: mensagemDeErro(b, resp.StatusCode)}
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
		e.RetryAfter = time.Duration(s) * time.Second
	}
	return nil, e
}

func mensagemDeErro(b []byte, status int) string {
	var v struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &v) == nil && v.Error.Message != "" {
		return v.Error.Message
	}
	return http.StatusText(status)
}
