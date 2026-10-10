// Package gateway reúne o domínio da Comporta: configuração de modelos e preços,
// cálculo de custo e o roteador que escolhe o destino de cada requisição.
package gateway

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"
)

// Config espelha o comporta.json. Modelos, preços e janelas de pico ficam fora do código
// porque mudam com frequência (a DeepSeek já trocou nomes e preços várias vezes).
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

// ModeloCfg é um alias público ("rapido", "forte"). O cliente nunca vê o nome real do modelo,
// então dá para trocar de provedor sem mudar nenhuma aplicação.
type ModeloCfg struct {
	Destinos     []Destino `json:"destinos"`
	Shadow       *Destino  `json:"shadow,omitempty"`
	Concorrencia int       `json:"concorrencia"`
}

type Destino struct {
	Provedor string `json:"provedor"`
	Modelo   string `json:"modelo"`
}

// todos devolve os destinos mais o shadow, sem alterar o slice da config.
func (m ModeloCfg) todos() []Destino {
	if m.Shadow == nil {
		return m.Destinos
	}
	return append(m.Destinos[:len(m.Destinos):len(m.Destinos)], *m.Shadow)
}

func (d Destino) String() string { return d.Provedor + "/" + d.Modelo }

// PrecoCfg vem em US$ por 1 milhão de tokens, como nas tabelas dos provedores.
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

// precoMicro guarda o preço em micro-dólares por 1M de tokens: dinheiro nunca vira float
// depois que a config é lida.
type precoMicro struct{ hit, miss, saida int64 }

func Carregar(caminho string) (*Config, error) {
	b, err := os.ReadFile(caminho)
	if err != nil {
		return nil, fmt.Errorf("lendo config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("decodificando config: %w", err)
	}
	if err := c.validar(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validar() error {
	if len(c.Modelos) == 0 {
		return fmt.Errorf("config: nenhum modelo")
	}
	// Zero = usar o padrão; negativo faria os prazos vencerem na hora e toda requisição dar 504.
	if t := c.Timeouts; min(t.ConexaoS, t.PrimeiroTokenS, t.EntreChunksS, t.TotalS) < 0 {
		return fmt.Errorf("config: timeout negativo em \"timeouts\"")
	}
	for alias, m := range c.Modelos {
		if len(m.Destinos) == 0 {
			return fmt.Errorf("config: modelo %q sem destinos", alias)
		}
		for _, d := range m.todos() {
			if _, ok := c.Provedores[d.Provedor]; !ok {
				return fmt.Errorf("config: modelo %q usa provedor desconhecido %q", alias, d.Provedor)
			}
			// Sem preço a chamada custaria 0 e nunca consumiria o orçamento do cliente.
			if _, ok := c.Precos[d.Modelo]; !ok {
				return fmt.Errorf("config: modelo %q usa %q sem preço em \"precos\"", alias, d.Modelo)
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
