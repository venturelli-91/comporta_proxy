package httpapi

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type contadores struct {
	requisicoes, erros, cacheHits, pii atomic.Int64
}

// metricas escreve no formato texto do Prometheus.
// ponytail: contadores próprios; o client oficial entra junto com labels por cliente/modelo.
func (s *Servidor) metricas(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	c := func(nome, ajuda string, v int64) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", nome, ajuda, nome, nome, v)
	}
	c("comporta_requisicoes_total", "Requisições de chat atendidas.", s.met.requisicoes.Load())
	c("comporta_erros_total", "Requisições de chat com status >= 400.", s.met.erros.Load())
	c("comporta_cache_hits_total", "Respostas servidas pelo cache ou pelo singleflight.", s.met.cacheHits.Load())
	c("comporta_pii_mascarados_total", "Dados pessoais mascarados antes de sair para o provedor.", s.met.pii.Load())
	c("comporta_uso_descartados_total", "Eventos de uso descartados com o canal cheio.", s.d.Gravador.Descartados())
	fmt.Fprintf(w, "# HELP comporta_cache_bytes Bytes ocupados pelo cache.\n# TYPE comporta_cache_bytes gauge\ncomporta_cache_bytes %d\n", s.d.Cache.Bytes())
	fmt.Fprint(w, "# HELP comporta_breaker_aberto 1 quando o circuit breaker do destino não está fechado.\n# TYPE comporta_breaker_aberto gauge\n")
	for _, e := range s.d.Roteador.Estado() {
		v := 0
		if e.Breaker != "closed" {
			v = 1
		}
		fmt.Fprintf(w, "comporta_breaker_aberto{destino=%q} %d\n", e.Destino, v)
	}
}
