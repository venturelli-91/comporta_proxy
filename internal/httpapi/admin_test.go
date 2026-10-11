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

// Going past 10k IPs must not give attempts back to whoever already spent theirs.
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

	s.limiteLogin("198.51.100.7") // 10,001 entries: triggers the pruning

	if s.tentativas["203.0.113.9"] != gasto {
		t.Fatal("the spent limiter was deleted: the attacker got fresh attempts")
	}
	if n := len(s.tentativas); n != 2 {
		t.Fatalf("%d limiters left; want only the spent one and the new one", n)
	}
}

// With the dashboard on, "/" catches everything: a wrong API route can't become index.html with a 200,
// or an SDK calling /v1/embeddings gets HTML and breaks with a confusing error.
func TestRotaDesconhecidaDaAPIDa404(t *testing.T) {
	c := novoCenario(t)
	c.s.d.Painel = fstest.MapFS{"index.html": {Data: []byte("<html>painel</html>")}}
	h := c.s.Handler()
	for _, caminho := range []string{"/v1/embeddings", "/admin/nada"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, caminho, nil))
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"rota_inexistente"`) {
			t.Errorf("%s: status %d, body %q; want a JSON 404", caminho, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/clientes", nil))
	if !strings.Contains(rec.Body.String(), "painel") {
		t.Errorf("dashboard route no longer falls back to index.html: %q", rec.Body.String())
	}
}

// With an empty token, iguais("", "") would be true and anyone could get into the admin.
func TestNovoRecusaTokenDeAdminVazio(t *testing.T) {
	if _, err := Novo(Deps{}); err == nil {
		t.Fatal("Novo accepted an empty AdminToken")
	}
}

// index.html is the only build file without a hash in its name and it points at the hashed bundle:
// if the browser caches it, a deploy keeps loading the old bundle. Hashed assets need no header.
func TestIndexHTMLNaoFicaEmCache(t *testing.T) {
	c := novoCenario(t)
	c.s.d.Painel = fstest.MapFS{
		"index.html":      {Data: []byte("<html>painel</html>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
	}
	h := c.s.Handler()
	for _, caminho := range []string{"/", "/clientes"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, caminho, nil))
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want no-cache", caminho, got)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app-1.js", nil))
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Errorf("hashed asset: Cache-Control = %q, want none", got)
	}
}
