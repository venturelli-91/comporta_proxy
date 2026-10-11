package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"golang.org/x/time/rate"
)

// Passar de 10 mil IPs não pode devolver tentativas a quem já gastou as suas.
func TestLimiteLoginPodaSoOsCheios(t *testing.T) {
	s := &Servidor{tentativas: map[string]*rate.Limiter{}}
	for i := range 10_000 {
		s.tentativas[strconv.Itoa(i)] = rate.NewLimiter(rate.Every(12*time.Second), 5)
	}
	gasto := rate.NewLimiter(rate.Every(12*time.Second), 5)
	for range 5 {
		gasto.Allow()
	}
	s.tentativas["203.0.113.9"] = gasto

	s.limiteLogin("198.51.100.7") // 10.001 entradas: dispara a poda

	if s.tentativas["203.0.113.9"] != gasto {
		t.Fatal("o limitador gasto foi apagado: o atacante ganhou tentativas novas")
	}
	if n := len(s.tentativas); n != 2 {
		t.Fatalf("sobraram %d limitadores; queria só o gasto e o novo", n)
	}
}

// Com o painel ligado, "/" pega tudo: uma rota errada da API não pode virar o index.html com 200,
// senão um SDK chamando /v1/embeddings recebe HTML e quebra com um erro confuso.
func TestRotaDesconhecidaDaAPIDa404(t *testing.T) {
	c := novoCenario(t)
	c.s.d.Painel = fstest.MapFS{"index.html": {Data: []byte("<html>painel</html>")}}
	h := c.s.Handler()
	for _, caminho := range []string{"/v1/embeddings", "/admin/nada"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, caminho, nil))
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"rota_inexistente"`) {
			t.Errorf("%s: status %d, corpo %q; queria 404 JSON", caminho, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/clientes", nil))
	if !strings.Contains(rec.Body.String(), "painel") {
		t.Errorf("rota do painel deixou de cair no index.html: %q", rec.Body.String())
	}
}

// Com token vazio, iguais("", "") seria verdade e qualquer um entraria no admin.
func TestNovoRecusaTokenDeAdminVazio(t *testing.T) {
	if _, err := Novo(Deps{}); err == nil {
		t.Fatal("Novo aceitou AdminToken vazio")
	}
}
