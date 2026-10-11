package httpapi

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type contadores struct {
	requisicoes, erros, cacheHits, pii, sombrasDescartadas atomic.Int64
}

// metricas writes the Prometheus text format.
// ponytail: hand-rolled counters; the official client comes in together with per-client/model labels.
func (s *Servidor) metricas(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	c := func(nome, ajuda string, v int64) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", nome, ajuda, nome, nome, v)
	}
	c("comporta_requisicoes_total", "Chat requests served.", s.met.requisicoes.Load())
	c("comporta_erros_total", "Chat requests with status >= 400.", s.met.erros.Load())
	c("comporta_cache_hits_total", "Responses served by the cache or the singleflight.", s.met.cacheHits.Load())
	c("comporta_pii_mascarados_total", "Personal data values masked before leaving for the provider.", s.met.pii.Load())
	c("comporta_shadow_descartados_total", "Shadow calls dropped for lack of a free slot.", s.met.sombrasDescartadas.Load())
	c("comporta_uso_descartados_total", "Usage events dropped because the channel was full.", s.d.Gravador.Descartados())
	fmt.Fprintf(w, "# HELP comporta_cache_bytes Bytes used by the cache.\n# TYPE comporta_cache_bytes gauge\ncomporta_cache_bytes %d\n", s.d.Cache.Bytes())
	fmt.Fprint(w, "# HELP comporta_breaker_aberto 1 when the destination's circuit breaker is not closed.\n# TYPE comporta_breaker_aberto gauge\n")
	for _, e := range s.d.Roteador.Estado() {
		v := 0
		if e.Breaker != "closed" {
			v = 1
		}
		fmt.Fprintf(w, "comporta_breaker_aberto{destino=%q} %d\n", e.Destino, v)
	}
}
