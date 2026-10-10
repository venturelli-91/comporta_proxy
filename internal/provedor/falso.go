package provedor

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Falso imita a DeepSeek no mesmo protocolo HTTP: testes e k6 passam pelo código de rede de
// verdade, sem custo e sem depender da internet. A resposta ecoa a última mensagem, o que
// deixa visível no teste o que o provedor recebeu (por exemplo, o CPF já mascarado).
type Falso struct {
	Latencia time.Duration // tempo até o primeiro token
	PorToken time.Duration
	TaxaErro float64 // 0..1, devolve 503
	Chamadas atomic.Int64
}

func (f *Falso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.Chamadas.Add(1)
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
		erroFalso(w, http.StatusNotFound, "rota inexistente")
		return
	}
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Stream        bool `json:"stream"`
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) == 0 {
		erroFalso(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if f.TaxaErro > 0 && rand.Float64() < f.TaxaErro {
		erroFalso(w, http.StatusServiceUnavailable, "servidor ocupado (falso)")
		return
	}

	var ultima string
	prompt := 0
	for _, m := range req.Messages {
		var s string
		_ = json.Unmarshal(m.Content, &s)
		prompt += len(s)/4 + 1
		ultima = s
	}
	texto := "Resposta do " + req.Model + ": " + ultima
	pedacos := strings.SplitAfter(texto, " ")
	uso := map[string]int{
		"prompt_tokens": prompt, "completion_tokens": len(pedacos), "total_tokens": prompt + len(pedacos),
		"prompt_cache_hit_tokens": 0, "prompt_cache_miss_tokens": prompt,
	}
	id := fmt.Sprintf("falso-%d", time.Now().UnixNano())
	ctx := r.Context()

	if !req.Stream {
		if !esperar(ctx, f.Latencia+time.Duration(len(pedacos))*f.PorToken) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": id, "object": "chat.completion", "created": time.Now().Unix(), "model": req.Model,
			"choices": []any{map[string]any{"index": 0, "finish_reason": "stop",
				"message": map[string]string{"role": "assistant", "content": texto}}},
			"usage": uso,
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	fl, _ := w.(http.Flusher)
	evento := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if fl != nil {
			fl.Flush()
		}
	}
	chunk := func(delta map[string]string, fim any) map[string]any {
		return map[string]any{"id": id, "object": "chat.completion.chunk", "model": req.Model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": fim}}}
	}
	// A DeepSeek manda comentários SSE enquanto a inferência não começa.
	fmt.Fprint(w, ": keep-alive\n\n")
	if fl != nil {
		fl.Flush()
	}
	if !esperar(ctx, f.Latencia) {
		return
	}
	evento(chunk(map[string]string{"role": "assistant", "content": ""}, nil))
	for _, p := range pedacos {
		if !esperar(ctx, f.PorToken) {
			return
		}
		evento(chunk(map[string]string{"content": p}, nil))
	}
	evento(chunk(map[string]string{}, "stop"))
	// Como a DeepSeek e a OpenAI: o usage vem num chunk próprio, com choices vazio.
	if req.StreamOptions.IncludeUsage {
		evento(map[string]any{"id": id, "object": "chat.completion.chunk", "model": req.Model, "choices": []any{}, "usage": uso})
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func esperar(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func erroFalso(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": msg}})
}
