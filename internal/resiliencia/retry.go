// Package resiliencia protege o gateway de provedores instáveis.
package resiliencia

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

// As interfaces ficam aqui, no pacote que as usa: o retry não precisa conhecer o provedor,
// só saber se vale tentar de novo e quanto o servidor pediu para esperar.
type repetivel interface{ Repetivel() bool }
type comEspera interface{ Espera() time.Duration }

// Retry chama fn até `tentativas` vezes. Só repete erros que se declaram repetíveis
// (429, 5xx, rede). A espera é exponencial com metade fixa e metade aleatória: o piso garante
// que o provedor respira, o jitter espalha as tentativas de muitos clientes para que não
// voltem todos juntos.
// Se o provedor mandou Retry-After, ele vale mais que o nosso cálculo.
// Se a espera não cabe no prazo do contexto, desiste já e devolve o último erro.
func Retry(ctx context.Context, tentativas int, base time.Duration, fn func() error) error {
	var err error
	for i := 0; i < tentativas; i++ {
		if err = fn(); err == nil {
			return nil
		}
		var r repetivel
		if !errors.As(err, &r) || !r.Repetivel() || i == tentativas-1 {
			return err
		}
		teto := base << i
		espera := teto/2 + time.Duration(rand.Int64N(int64(teto/2)+1))
		var e comEspera
		if errors.As(err, &e) && e.Espera() > 0 {
			espera = e.Espera()
		}
		if prazo, ok := ctx.Deadline(); ok && time.Until(prazo) < espera {
			return err
		}
		t := time.NewTimer(espera)
		select {
		case <-ctx.Done():
			t.Stop()
			return err
		case <-t.C:
		}
	}
	return err
}
