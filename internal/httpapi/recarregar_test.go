package httpapi

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/venturelli-91/comporta_proxy/internal/armazenamento"
)

// Um Recarregar que leu o banco antes da revogação não pode gravar o snapshot depois de
// quem leu depois dela: a chave revogada voltaria a valer até a próxima mutação.
// O gancho segura o primeiro Recarregar entre a leitura e a troca, onde a corrida acontece.
func TestRecarregarConcorrenteNaoDesfazRevogacao(t *testing.T) {
	c := novoCenario(t)
	ctx := context.Background()
	cli, _ := c.banco.CriarCliente(ctx, armazenamento.Cliente{Nome: "loja"})
	texto, k, _ := c.banco.CriarChave(ctx, cli.ID)

	leu, solta := make(chan struct{}), make(chan struct{})
	// Contador e não sync.Once: o Once faria a segunda chamada esperar a primeira, escondendo a corrida.
	var chamadas atomic.Int32
	c.s.antesDeTrocar = func() {
		if chamadas.Add(1) == 1 { // só o primeiro fica preso
			close(leu)
			<-solta
		}
	}

	var wg sync.WaitGroup
	wg.Go(func() { _ = c.s.Recarregar(ctx) }) // leu com a chave ainda ativa
	<-leu
	if err := c.banco.RevogarChave(ctx, cli.ID, k.ID); err != nil {
		t.Fatal(err)
	}
	segundo := make(chan struct{})
	wg.Go(func() { _ = c.s.Recarregar(ctx); close(segundo) }) // o recarregar do handler de revogar
	// Sem exclusão o segundo termina já; com ela fica esperando o primeiro. Dá um tempo e solta.
	select {
	case <-segundo:
	case <-time.After(200 * time.Millisecond):
	}
	close(solta)
	wg.Wait()

	if _, ativa := c.s.snap.Load().chaves[armazenamento.HashChave(texto)]; ativa {
		t.Fatal("chave revogada voltou ao snapshot por um Recarregar mais antigo")
	}
}
