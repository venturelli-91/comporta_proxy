package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCarregar(t *testing.T) {
	casos := []struct {
		nome, json, querErro string
	}{
		{"valid config", `{"provedores":{"p":{}},"modelos":{"a":{"destinos":[{"provedor":"p","modelo":"m"}]}},"precos":{"m":{"saida":1}}}`, ""},
		// A model without a price would cost 0 and never consume the client's budget: loading fails.
		{"destination without price", `{"provedores":{"p":{}},"modelos":{"a":{"destinos":[{"provedor":"p","modelo":"m"}]}}}`, "no price"},
		{"shadow without price", `{"provedores":{"p":{}},"modelos":{"a":{"destinos":[{"provedor":"p","modelo":"m"}],"shadow":{"provedor":"p","modelo":"s"}}},"precos":{"m":{"saida":1}}}`, "no price"},
		{"unknown provider", `{"provedores":{},"modelos":{"a":{"destinos":[{"provedor":"x","modelo":"m"}]}},"precos":{"m":{"saida":1}}}`, "unknown provider"},
		// A negative timeout would fire the watchdog immediately: every request would become a 504.
		{"negative timeout", `{"provedores":{"p":{}},"modelos":{"a":{"destinos":[{"provedor":"p","modelo":"m"}]}},"precos":{"m":{"saida":1}},"timeouts":{"primeiro_token_s":-1}}`, "negative timeout"},
		{"alias without destination", `{"provedores":{"p":{}},"modelos":{"a":{"destinos":[]}}}`, "no destinations"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			caminho := filepath.Join(t.TempDir(), "c.json")
			if err := os.WriteFile(caminho, []byte(c.json), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Carregar(caminho)
			if c.querErro == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.querErro != "" && (err == nil || !strings.Contains(err.Error(), c.querErro)) {
				t.Fatalf("error = %v, want it to contain %q", err, c.querErro)
			}
		})
	}
}

// The repository's comporta.json must stay valid.
func TestConfigDoRepositorio(t *testing.T) {
	if _, err := Carregar("../../comporta.json"); err != nil {
		t.Fatal(err)
	}
}

// Zero means "use the default": the config reaches its users already filled in, in one place.
// Without this, a missing conexao_s became net.Dialer{Timeout: 0}, that is, no connection deadline.
func TestCarregarPreenchePadroes(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "c.json")
	json := `{"provedores":{"p":{}},"modelos":{"a":{"destinos":[{"provedor":"p","modelo":"m"}]}},"precos":{"m":{"saida":1}},"timeouts":{"total_s":42}}`
	if err := os.WriteFile(caminho, []byte(json), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Carregar(caminho)
	if err != nil {
		t.Fatal(err)
	}
	quer := TimeoutsCfg{ConexaoS: 5, PrimeiroTokenS: 30, EntreChunksS: 60, TotalS: 42} // what came in the JSON is kept
	if c.Timeouts != quer {
		t.Errorf("timeouts = %+v, want %+v", c.Timeouts, quer)
	}
	if c.Cache != (CacheCfg{MaxMB: 64, TTLs: 300}) {
		t.Errorf("cache = %+v, want {MaxMB:64 TTLs:300}", c.Cache)
	}
}
