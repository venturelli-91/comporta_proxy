package gateway

import "time"

// Uso é o bloco "usage" da API compatível com OpenAI, com os campos extras da DeepSeek
// (cache de prefixo). Os tokens de raciocínio já estão dentro de CompletionTokens.
type Uso struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	CacheHitTokens   int64 `json:"prompt_cache_hit_tokens"`
	CacheMissTokens  int64 `json:"prompt_cache_miss_tokens"`
	Detalhes         struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

// Custo devolve o custo em micro-dólares. Carregar já recusa modelo de destino sem preço
// (custaria 0 e furaria o orçamento); o 0 aqui só aparece em configs montadas à mão nos testes.
func (c *Config) Custo(modelo string, u Uso, t time.Time) int64 {
	p, ok := c.preco(modelo)
	if !ok {
		return 0
	}
	hit, miss := u.CacheHitTokens, u.CacheMissTokens
	if hit+miss == 0 {
		miss = u.PromptTokens
	}
	// Soma antes de dividir para não perder frações em requisições pequenas.
	total := hit*p.hit + miss*p.miss + u.CompletionTokens*p.saida
	micro := (total + 500_000) / 1_000_000
	if !c.Pico.emPico(t) {
		micro = micro * (100 - c.Pico.DescontoForaPicoPct) / 100
	}
	return micro
}

// emPico: sem janelas configuradas, tudo é preço cheio.
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
