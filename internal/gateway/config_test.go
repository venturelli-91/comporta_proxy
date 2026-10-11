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
		{"config válida", `{"provedores":{"p":{}},"modelos":{"a":{"destinos":[{"provedor":"p","modelo":"m"}]}},"precos":{"m":{"saida":1}}}`, ""},
		// Modelo sem preço custaria 0 e nunca consumiria o orçamento do cliente: falha na carga.
		{"destino sem preço", `{"provedores":{"p":{}},"modelos":{"a":{"destinos":[{"provedor":"p","modelo":"m"}]}}}`, "sem preço"},
		{"shadow sem preço", `{"provedores":{"p":{}},"modelos":{"a":{"destinos":[{"provedor":"p","modelo":"m"}],"shadow":{"provedor":"p","modelo":"s"}}},"precos":{"m":{"saida":1}}}`, "sem preço"},
		{"provedor desconhecido", `{"provedores":{},"modelos":{"a":{"destinos":[{"provedor":"x","modelo":"m"}]}},"precos":{"m":{"saida":1}}}`, "provedor desconhecido"},
		// Timeout negativo faria o watchdog disparar na hora: toda requisição viraria 504.
		{"timeout negativo", `{"provedores":{"p":{}},"modelos":{"a":{"destinos":[{"provedor":"p","modelo":"m"}]}},"precos":{"m":{"saida":1}},"timeouts":{"primeiro_token_s":-1}}`, "timeout negativo"},
		{"alias sem destino", `{"provedores":{"p":{}},"modelos":{"a":{"destinos":[]}}}`, "sem destinos"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			caminho := filepath.Join(t.TempDir(), "c.json")
			if err := os.WriteFile(caminho, []byte(c.json), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Carregar(caminho)
			if c.querErro == "" && err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if c.querErro != "" && (err == nil || !strings.Contains(err.Error(), c.querErro)) {
				t.Fatalf("erro = %v, queria conter %q", err, c.querErro)
			}
		})
	}
}

// O comporta.json do repositório precisa continuar válido.
func TestConfigDoRepositorio(t *testing.T) {
	if _, err := Carregar("../../comporta.json"); err != nil {
		t.Fatal(err)
	}
}

// Zero significa "use o padrão": a config chega a quem usa já preenchida, num lugar só.
// Sem isso, conexao_s ausente virava net.Dialer{Timeout: 0}, ou seja, sem prazo de conexão.
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
	quer := TimeoutsCfg{ConexaoS: 5, PrimeiroTokenS: 30, EntreChunksS: 60, TotalS: 42} // o que veio no JSON é mantido
	if c.Timeouts != quer {
		t.Errorf("timeouts = %+v, queria %+v", c.Timeouts, quer)
	}
	if c.Cache != (CacheCfg{MaxMB: 64, TTLs: 300}) {
		t.Errorf("cache = %+v, queria {MaxMB:64 TTLs:300}", c.Cache)
	}
}
