package httpapi

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/venturelli-91/comporta_proxy/internal/armazenamento"
)

// A Recarregar that read the database before the revocation can't store its snapshot after one that
// read after it: the revoked key would work again until the next mutation.
// The hook holds the first Recarregar between the read and the swap, where the race happens.
func TestRecarregarConcorrenteNaoDesfazRevogacao(t *testing.T) {
	c := novoCenario(t)
	ctx := context.Background()
	cli, err := c.banco.CriarCliente(ctx, armazenamento.Cliente{Nome: "loja"})
	if err != nil {
		t.Fatal(err)
	}
	texto, k, err := c.banco.CriarChave(ctx, cli.ID)
	if err != nil {
		t.Fatal(err)
	}

	leu, solta := make(chan struct{}), make(chan struct{})
	// A counter, not sync.Once: Once would make the second call wait for the first, hiding the race.
	var chamadas atomic.Int32
	c.s.antesDeTrocar = func() {
		if chamadas.Add(1) == 1 { // only the first one is held
			close(leu)
			<-solta
		}
	}

	var wg sync.WaitGroup
	wg.Go(func() { _ = c.s.Recarregar(ctx) }) // read with the key still active
	<-leu
	if err := c.banco.RevogarChave(ctx, cli.ID, k.ID); err != nil {
		t.Fatal(err)
	}
	segundo := make(chan struct{})
	wg.Go(func() { _ = c.s.Recarregar(ctx); close(segundo) }) // the revoke handler's reload
	// Without mutual exclusion the second finishes right away; with it, it waits for the first. Wait a bit and release.
	select {
	case <-segundo:
	case <-time.After(200 * time.Millisecond):
	}
	close(solta)
	wg.Wait()

	if _, ativa := c.s.snap.Load().chaves[armazenamento.HashChave(texto)]; ativa {
		t.Fatal("revoked key came back to the snapshot through an older Recarregar")
	}
}
