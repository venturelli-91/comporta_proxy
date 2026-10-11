package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/venturelli-91/comporta_proxy/internal/armazenamento"
)

// BenchmarkOverhead measures the gateway's cost per request (auth, admission, decode, LGPD,
// router, relay and usage record) with a zero-latency fake provider on loopback.
//
//	go test -run x -bench Overhead -benchmem ./internal/httpapi/
func BenchmarkOverhead(b *testing.B) {
	for _, caso := range []struct {
		nome string
		cli  armazenamento.Cliente
	}{
		{"direto", armazenamento.Cliente{Nome: "b"}},
		{"lgpd", armazenamento.Cliente{Nome: "b", LGPD: true}},
	} {
		b.Run(caso.nome, func(b *testing.B) {
			c := novoCenario(b)
			_, chave := c.novoCliente(caso.cli)
			corpo := msg("rapido", "meu CPF é 529.982.247-25, pode confirmar o cadastro?", false)
			h := c.s.Handler()
			b.ReportAllocs()
			for b.Loop() {
				req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(corpo))
				req.Header.Set("Authorization", "Bearer "+chave)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, req)
				if w.Code != http.StatusOK {
					b.Fatalf("status %d: %s", w.Code, w.Body)
				}
			}
		})
	}
}
