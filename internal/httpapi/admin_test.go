package httpapi

import (
	"strconv"
	"testing"
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
