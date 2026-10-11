// Package gateway holds Comporta's domain: model and price configuration,
// cost calculation and the router that picks each request's destination.
package gateway

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"
)

// Config mirrors comporta.json. Models, prices and peak windows live outside the code
// because they change often (DeepSeek has renamed and repriced models several times).
type Config struct {
	Provedores map[string]ProvedorCfg `json:"provedores"`
	Modelos    map[string]ModeloCfg   `json:"modelos"`
	Precos     map[string]PrecoCfg    `json:"precos"`
	Pico       PicoCfg                `json:"pico"`
	Cache      CacheCfg               `json:"cache"`
	Timeouts   TimeoutsCfg            `json:"timeouts"`
}

type ProvedorCfg struct {
	BaseURL     string `json:"base_url"`
	ChaveEnv    string `json:"chave_env"`
	MaxConexoes int    `json:"max_conexoes"`
}

// ModeloCfg is a public alias ("rapido", "forte"). The client never sees the real model name,
// so the provider can change without touching any application.
type ModeloCfg struct {
	Destinos     []Destino `json:"destinos"`
	Shadow       *Destino  `json:"shadow,omitempty"`
	Concorrencia int       `json:"concorrencia"`
}

type Destino struct {
	Provedor string `json:"provedor"`
	Modelo   string `json:"modelo"`
}

// todos returns the destinations plus the shadow, without changing the config's slice.
func (m ModeloCfg) todos() []Destino {
	if m.Shadow == nil {
		return m.Destinos
	}
	return append(m.Destinos[:len(m.Destinos):len(m.Destinos)], *m.Shadow)
}

func (d Destino) String() string { return d.Provedor + "/" + d.Modelo }

// PrecoCfg is in US$ per 1 million tokens, as in the providers' price tables.
type PrecoCfg struct {
	EntradaCacheHit  float64 `json:"entrada_cache_hit"`
	EntradaCacheMiss float64 `json:"entrada_cache_miss"`
	Saida            float64 `json:"saida"`
}

type PicoCfg struct {
	JanelasUTC          [][2]int `json:"janelas_utc"`
	SoDiasUteis         bool     `json:"so_dias_uteis"`
	DescontoForaPicoPct int64    `json:"desconto_fora_pico_pct"`
}

type CacheCfg struct {
	MaxMB int64 `json:"max_mb"`
	TTLs  int   `json:"ttl_s"`
}

type TimeoutsCfg struct {
	ConexaoS       int `json:"conexao_s"`
	PrimeiroTokenS int `json:"primeiro_token_s"`
	EntreChunksS   int `json:"entre_chunks_s"`
	TotalS         int `json:"total_s"`
}

func seg(n int) time.Duration { return time.Duration(n) * time.Second }

func (t TimeoutsCfg) Conexao() time.Duration       { return seg(t.ConexaoS) }
func (t TimeoutsCfg) PrimeiroToken() time.Duration { return seg(t.PrimeiroTokenS) }
func (t TimeoutsCfg) EntreChunks() time.Duration   { return seg(t.EntreChunksS) }
func (t TimeoutsCfg) Total() time.Duration         { return seg(t.TotalS) }

// precoMicro stores the price in micro-dollars per 1M tokens: money never becomes a float
// after the config is read.
type precoMicro struct{ hit, miss, saida int64 }

func Carregar(caminho string) (*Config, error) {
	b, err := os.ReadFile(caminho)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("decoding config: %w", err)
	}
	if err := c.validar(); err != nil {
		return nil, err
	}
	c.preencherPadroes()
	return &c, nil
}

// preencherPadroes replaces zeros with the defaults, in one place: whoever uses the config gets it
// complete (before, a missing conexao_s meant a dial without a timeout).
func (c *Config) preencherPadroes() {
	padrao := func(v *int, p int) {
		if *v == 0 {
			*v = p
		}
	}
	t := &c.Timeouts
	padrao(&t.ConexaoS, 5)
	padrao(&t.PrimeiroTokenS, 30)
	padrao(&t.EntreChunksS, 60)
	padrao(&t.TotalS, 600)
	padrao(&c.Cache.TTLs, 300)
	if c.Cache.MaxMB == 0 {
		c.Cache.MaxMB = 64
	}
}

func (c *Config) validar() error {
	if len(c.Modelos) == 0 {
		return fmt.Errorf("config: no models")
	}
	// Zero = use the default; a negative value would make deadlines expire immediately and every request return 504.
	if t := c.Timeouts; min(t.ConexaoS, t.PrimeiroTokenS, t.EntreChunksS, t.TotalS) < 0 {
		return fmt.Errorf("config: negative timeout in \"timeouts\"")
	}
	if c.Cache.MaxMB < 0 || c.Cache.TTLs < 0 {
		return fmt.Errorf("config: negative value in \"cache\"")
	}
	for alias, m := range c.Modelos {
		if len(m.Destinos) == 0 {
			return fmt.Errorf("config: model %q has no destinations", alias)
		}
		for _, d := range m.todos() {
			if _, ok := c.Provedores[d.Provedor]; !ok {
				return fmt.Errorf("config: model %q uses unknown provider %q", alias, d.Provedor)
			}
			// Without a price the call would cost 0 and never consume the client's budget.
			if _, ok := c.Precos[d.Modelo]; !ok {
				return fmt.Errorf("config: model %q uses %q, which has no price in \"precos\"", alias, d.Modelo)
			}
		}
	}
	return nil
}

func (c *Config) preco(modelo string) (precoMicro, bool) {
	p, ok := c.Precos[modelo]
	if !ok {
		return precoMicro{}, false
	}
	m := func(usd float64) int64 { return int64(math.Round(usd * 1e6)) }
	return precoMicro{m(p.EntradaCacheHit), m(p.EntradaCacheMiss), m(p.Saida)}, true
}
