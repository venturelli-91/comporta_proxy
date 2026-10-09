package gateway

import (
	"testing"
	"time"
)

func TestCusto(t *testing.T) {
	cfg := &Config{
		Precos: map[string]PrecoCfg{"m": {EntradaCacheHit: 0.1, EntradaCacheMiss: 1, Saida: 2}},
		Pico:   PicoCfg{JanelasUTC: [][2]int{{6, 10}}, SoDiasUteis: true, DescontoForaPicoPct: 50},
	}
	quintaPico := time.Date(2026, 10, 8, 7, 0, 0, 0, time.UTC)
	quintaForaPico := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	sabadoPico := time.Date(2026, 10, 10, 7, 0, 0, 0, time.UTC)

	casos := []struct {
		nome   string
		modelo string
		uso    Uso
		quando time.Time
		quer   int64
	}{
		{"pico sem cache de prefixo", "m", Uso{PromptTokens: 1000, CompletionTokens: 500}, quintaPico, 2000},
		{"fora do pico paga metade", "m", Uso{PromptTokens: 1000, CompletionTokens: 500}, quintaForaPico, 1000},
		{"fim de semana não é pico", "m", Uso{PromptTokens: 1000, CompletionTokens: 500}, sabadoPico, 1000},
		{"cache de prefixo barateia a entrada", "m", Uso{PromptTokens: 1000, CacheHitTokens: 800, CacheMissTokens: 200, CompletionTokens: 0}, quintaPico, 280},
		{"requisição pequena arredonda, não zera", "m", Uso{PromptTokens: 1, CompletionTokens: 1}, quintaPico, 3},
		{"modelo sem preço custa zero", "x", Uso{PromptTokens: 1000}, quintaPico, 0},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := cfg.Custo(c.modelo, c.uso, c.quando); got != c.quer {
				t.Errorf("Custo = %d, quer %d", got, c.quer)
			}
		})
	}
}

func TestCarregarValida(t *testing.T) {
	cfg := &Config{
		Provedores: map[string]ProvedorCfg{"p": {}},
		Modelos:    map[string]ModeloCfg{"a": {Destinos: []Destino{{"p", "m"}}, Shadow: &Destino{"nao-existe", "m"}}},
	}
	if err := cfg.validar(); err == nil {
		t.Fatal("esperava erro para shadow com provedor desconhecido")
	}
	cfg.Modelos["a"] = ModeloCfg{}
	if err := cfg.validar(); err == nil {
		t.Fatal("esperava erro para modelo sem destinos")
	}
}

func TestComportaJSON(t *testing.T) {
	if _, err := Carregar("../../comporta.json"); err != nil {
		t.Fatal(err)
	}
}
