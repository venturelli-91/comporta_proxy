package gateway

import "time"

// Uso is the OpenAI-compatible API's "usage" block, with DeepSeek's extra fields
// (prefix cache). Reasoning tokens are already included in CompletionTokens.
type Uso struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	CacheHitTokens   int64 `json:"prompt_cache_hit_tokens"`
	CacheMissTokens  int64 `json:"prompt_cache_miss_tokens"`
	Detalhes         struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

// Custo returns the cost in micro-dollars. Carregar already refuses destination models without a price
// (they would cost 0 and bypass the budget); the 0 here only shows up in hand-built test configs.
func (c *Config) Custo(modelo string, u Uso, t time.Time) int64 {
	p, ok := c.preco(modelo)
	if !ok {
		return 0
	}
	hit, miss := u.CacheHitTokens, u.CacheMissTokens
	if hit+miss == 0 {
		miss = u.PromptTokens
	}
	// Sum before dividing so small requests don't lose their fractions.
	total := hit*p.hit + miss*p.miss + u.CompletionTokens*p.saida
	micro := (total + 500_000) / 1_000_000
	if !c.Pico.emPico(t) {
		micro = micro * (100 - c.Pico.DescontoForaPicoPct) / 100
	}
	return micro
}

// emPico (at peak): without configured windows, everything is full price.
func (p PicoCfg) emPico(t time.Time) bool {
	if len(p.JanelasUTC) == 0 {
		return true
	}
	t = t.UTC()
	if p.SoDiasUteis && (t.Weekday() == time.Saturday || t.Weekday() == time.Sunday) {
		return false
	}
	h := t.Hour()
	for _, j := range p.JanelasUTC {
		if h >= j[0] && h < j[1] {
			return true
		}
	}
	return false
}
