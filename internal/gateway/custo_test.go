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
		{"peak without prefix cache", "m", Uso{PromptTokens: 1000, CompletionTokens: 500}, quintaPico, 2000},
		{"off-peak pays half", "m", Uso{PromptTokens: 1000, CompletionTokens: 500}, quintaForaPico, 1000},
		{"weekend is not peak", "m", Uso{PromptTokens: 1000, CompletionTokens: 500}, sabadoPico, 1000},
		{"prefix cache makes input cheaper", "m", Uso{PromptTokens: 1000, CacheHitTokens: 800, CacheMissTokens: 200, CompletionTokens: 0}, quintaPico, 280},
		{"small request rounds, not zeroes", "m", Uso{PromptTokens: 1, CompletionTokens: 1}, quintaPico, 3},
		{"model without price costs zero", "x", Uso{PromptTokens: 1000}, quintaPico, 0},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := cfg.Custo(c.modelo, c.uso, c.quando); got != c.quer {
				t.Errorf("Custo = %d, want %d", got, c.quer)
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
		t.Fatal("expected an error for a shadow with an unknown provider")
	}
	cfg.Modelos["a"] = ModeloCfg{}
	if err := cfg.validar(); err == nil {
		t.Fatal("expected an error for a model without destinations")
	}
}

func TestComportaJSON(t *testing.T) {
	if _, err := Carregar("../../comporta.json"); err != nil {
		t.Fatal(err)
	}
}
