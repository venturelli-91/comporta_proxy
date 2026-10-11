// Package resiliencia (resilience) protects the gateway from unstable providers.
package resiliencia

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

// The interfaces live here, in the package that uses them: the retry doesn't need to know the provider,
// only whether retrying is worth it and how long the server asked to wait.
type repetivel interface{ Repetivel() bool }
type comEspera interface{ Espera() time.Duration }

// Retry calls fn up to `tentativas` (attempts) times. It only retries errors that declare themselves
// retryable (429, 5xx, network). The wait is exponential, half fixed and half random: the floor gives
// the provider room to breathe, the jitter spreads many clients' retries so they don't all come back
// at once.
// If the provider sent Retry-After, it wins over our calculation.
// If the wait doesn't fit in the context's deadline, it gives up right away and returns the last error.
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
