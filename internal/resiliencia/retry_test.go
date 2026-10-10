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
		erros        []error // erro devolvido em cada tentativa; acabou a lista → sucesso
		tentativas   int
		querErr      bool
		querChamadas int
		querMinimo   time.Duration
	}{
		{"sucesso na primeira", nil, 3, false, 1, 0},
		{"sucesso na terceira", []error{repetivel, repetivel}, 3, false, 3, 0},
		{"esgota as tentativas", []error{repetivel, repetivel, repetivel}, 3, true, 3, 0},
		{"erro não repetível sai na primeira", []error{erroTeste{}}, 3, true, 1, 0},
		{"erro comum não é repetido", []error{errors.New("x")}, 3, true, 1, 0},
		{"Retry-After respeitado", []error{erroTeste{repetivel: true, espera: 50 * time.Millisecond}}, 3, false, 2, 50 * time.Millisecond},
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
				t.Fatalf("err = %v, queria erro = %v", err, c.querErr)
			}
			if chamadas != c.querChamadas {
				t.Errorf("chamadas = %d, queria %d", chamadas, c.querChamadas)
			}
			if d := time.Since(inicio); d < c.querMinimo {
				t.Errorf("esperou %v, queria pelo menos %v", d, c.querMinimo)
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
		t.Fatalf("queria o último erro da chamada, veio %v", err)
	}
	if chamadas != 1 || time.Since(inicio) > 500*time.Millisecond {
		t.Fatalf("chamadas = %d em %v: não deveria esperar além do prazo", chamadas, time.Since(inicio))
	}
}
