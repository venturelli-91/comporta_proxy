// Package provedor (provider) talks to any OpenAI-compatible API (DeepSeek is one of them).
// That's why there's a single Cliente: "deepseek" and "openaicompat" differ only in base_url.
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

// Novo creates a client with its own transport. Go's default keeps only 2 idle connections
// per host: under load that forces new TLS handshakes all the time.
func Novo(baseURL, chave string, maxConexoes int, timeoutConexao time.Duration) *Cliente {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = maxConexoes
	t.MaxIdleConnsPerHost = maxConexoes
	t.MaxConnsPerHost = maxConexoes
	t.IdleConnTimeout = 90 * time.Second
	t.ForceAttemptHTTP2 = true
	t.DialContext = (&net.Dialer{Timeout: timeoutConexao, KeepAlive: 30 * time.Second}).DialContext
	// No http.Client.Timeout: it would cut long streams. Deadlines come from the context.
	return &Cliente{baseURL: baseURL, chave: chave, http: &http.Client{Transport: t}}
}

// ErroHTTP is a provider failure. Status 0 means a network error (no response).
type ErroHTTP struct {
	Status     int
	RetryAfter time.Duration
	Mensagem   string
	causa      error
}

func (e *ErroHTTP) Error() string {
	if e.Status == 0 {
		return "provider unreachable: " + e.Mensagem
	}
	return fmt.Sprintf("provider responded %d: %s", e.Status, e.Mensagem)
}

func (e *ErroHTTP) Unwrap() error { return e.causa }

// Repetivel (retryable): retrying only makes sense when the problem is the provider's (overload, 5xx, network).
// 400/401/402/422 will fail the same way on the next attempt.
func (e *ErroHTTP) Repetivel() bool { return e.Status == 0 || e.Status == 429 || e.Status >= 500 }

func (e *ErroHTTP) Espera() time.Duration { return e.RetryAfter }

// Enviar (send) makes the POST and returns the response only when it's 2xx; the caller closes the Body.
func (c *Cliente) Enviar(ctx context.Context, corpo []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(corpo))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.chave != "" {
		req.Header.Set("Authorization", "Bearer "+c.chave)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			// The cause says who cut it: the client (Canceled) or a gateway deadline.
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
