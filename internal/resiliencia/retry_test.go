package resiliencia

import (
	"context"
	"errors"
	"testing"
	"time"
)

type erroTeste struct {
	repetivel bool
	espera    time.Duration
}

func (e erroTeste) Error() string         { return "falhou" }
func (e erroTeste) Repetivel() bool       { return e.repetivel }
func (e erroTeste) Espera() time.Duration { return e.espera }

func TestRetry(t *testing.T) {
	repetivel := erroTeste{repetivel: true}
	casos := []struct {
		nome         string
		erros        []error // error returned on each attempt; end of the list → success
		tentativas   int
		querErr      bool
		querChamadas int
		querMinimo   time.Duration
	}{
		{"success on the first", nil, 3, false, 1, 0},
		{"success on the third", []error{repetivel, repetivel}, 3, false, 3, 0},
		{"exhausts the attempts", []error{repetivel, repetivel, repetivel}, 3, true, 3, 0},
		{"non-retryable error exits on the first", []error{erroTeste{}}, 3, true, 1, 0},
		{"plain error is not retried", []error{errors.New("x")}, 3, true, 1, 0},
		{"Retry-After honored", []error{erroTeste{repetivel: true, espera: 50 * time.Millisecond}}, 3, false, 2, 50 * time.Millisecond},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			chamadas := 0
			inicio := time.Now()
			err := Retry(context.Background(), c.tentativas, time.Millisecond, func() error {
				chamadas++
				if chamadas <= len(c.erros) {
					return c.erros[chamadas-1]
				}
				return nil
			})
			if (err != nil) != c.querErr {
				t.Fatalf("err = %v, want error = %v", err, c.querErr)
			}
			if chamadas != c.querChamadas {
				t.Errorf("calls = %d, want %d", chamadas, c.querChamadas)
			}
			if d := time.Since(inicio); d < c.querMinimo {
				t.Errorf("waited %v, want at least %v", d, c.querMinimo)
			}
		})
	}
}

func TestRetryContextoCancelado(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	chamadas := 0
	inicio := time.Now()
	err := Retry(ctx, 5, time.Second, func() error {
		chamadas++
		return erroTeste{repetivel: true}
	})
	if !errors.As(err, new(erroTeste)) {
		t.Fatalf("want the call's last error, got %v", err)
	}
	if chamadas != 1 || time.Since(inicio) > 500*time.Millisecond {
		t.Fatalf("calls = %d in %v: should not wait past the deadline", chamadas, time.Since(inicio))
	}
}
