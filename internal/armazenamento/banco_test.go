package armazenamento

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func abrir(t *testing.T) *Banco {
	t.Helper()
	b, err := Abrir(filepath.Join(t.TempDir(), "teste.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Fechar() })
	return b
}

var ctx = context.Background()

func TestClientesCRUD(t *testing.T) {
	b := abrir(t)
	c, err := b.CriarCliente(ctx, Cliente{Nome: "loja", RPM: 60, OrcamentoMicro: 5_000_000, Cache: true, LGPD: true, Modelos: []string{"rapido"}})
	if err != nil || c.ID == 0 {
		t.Fatalf("CriarCliente = %+v, %v", c, err)
	}
	c.RPM = 120
	c.Modelos = []string{"rapido", "forte"}
	if err := b.AtualizarCliente(ctx, c); err != nil {
		t.Fatal(err)
	}
	lista, err := b.ListarClientes(ctx)
	if err != nil || len(lista) != 1 || lista[0].RPM != 120 || len(lista[0].Modelos) != 2 || !lista[0].LGPD {
		t.Fatalf("ListarClientes = %+v, %v", lista, err)
	}
	if err := b.RemoverCliente(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if lista, _ := b.ListarClientes(ctx); len(lista) != 0 {
		t.Fatalf("cliente removido continua na lista: %+v", lista)
	}
	if err := b.AtualizarCliente(ctx, c); err == nil {
		t.Fatal("atualizar cliente inexistente devia dar erro")
	}
}

func TestChaveGuardadaSoComoHash(t *testing.T) {
	b := abrir(t)
	c, _ := b.CriarCliente(ctx, Cliente{Nome: "app"})
	texto, chave, err := b.CriarChave(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(texto, "ck_") || len(texto) != 29 || !strings.HasPrefix(texto, strings.TrimSuffix(chave.Prefixo, "…")) {
		t.Fatalf("texto = %q, prefixo = %q", texto, chave.Prefixo)
	}
	var n int
	if err := b.db.QueryRow(`SELECT count(*) FROM chaves WHERE hash = ? AND hash <> ?`, HashChave(texto), texto).Scan(&n); err != nil || n != 1 {
		t.Fatalf("hash não encontrado (n=%d, err=%v)", n, err)
	}
	if err := b.db.QueryRow(`SELECT count(*) FROM chaves WHERE hash = ? OR prefixo = ?`, texto, texto).Scan(&n); err != nil || n != 0 {
		t.Fatal("a chave inteira não pode estar gravada")
	}
	ativas, _ := b.ChavesAtivas(ctx)
	if ativas[HashChave(texto)] != c.ID {
		t.Fatalf("ChavesAtivas = %v", ativas)
	}
}

func TestRevogarChave(t *testing.T) {
	b := abrir(t)
	c, _ := b.CriarCliente(ctx, Cliente{Nome: "app"})
	_, chave, _ := b.CriarChave(ctx, c.ID)
	outro, _ := b.CriarCliente(ctx, Cliente{Nome: "outro"})
	if err := b.RevogarChave(ctx, outro.ID, chave.ID); !errors.Is(err, ErrNaoEncontrado) {
		t.Fatalf("revogar pela rota de outro cliente: err = %v, queria ErrNaoEncontrado", err)
	}
	if ativas, _ := b.ChavesAtivas(ctx); len(ativas) != 1 {
		t.Fatal("a chave foi revogada pela rota de outro cliente")
	}
	if err := b.RevogarChave(ctx, c.ID, chave.ID); err != nil {
		t.Fatal(err)
	}
	if ativas, _ := b.ChavesAtivas(ctx); len(ativas) != 0 {
		t.Fatalf("chave revogada continua ativa: %v", ativas)
	}
	lista, _ := b.ListarChaves(ctx, c.ID)
	if len(lista) != 1 || lista[0].RevogadaEm == nil {
		t.Fatalf("ListarChaves = %+v", lista)
	}
}

func TestGravadorEmLote(t *testing.T) {
	b := abrir(t)
	g := NovoGravador(b, 100)
	ctxRodar, parar := context.WithCancel(ctx)
	fim := make(chan struct{})
	go func() { g.Rodar(ctxRodar); close(fim) }()
	agora := time.Now()
	for i := range 3 {
		if !g.Registrar(Evento{TS: agora, ClienteID: 1, Modelo: "rapido", Status: 200, LatenciaMs: int64(i)}) {
			t.Fatal("Registrar recusou com canal vazio")
		}
	}
	parar() // o shutdown precisa drenar o que ficou no canal
	<-fim
	var n int
	_ = b.db.QueryRow(`SELECT count(*) FROM uso`).Scan(&n)
	if n != 3 {
		t.Fatalf("gravou %d eventos, queria 3", n)
	}
}

func TestGravadorCheioDescartaSemTravar(t *testing.T) {
	g := NovoGravador(abrir(t), 1)
	if !g.Registrar(Evento{}) {
		t.Fatal("primeiro evento devia entrar")
	}
	feito := make(chan bool)
	go func() { feito <- g.Registrar(Evento{}) }()
	select {
	case ok := <-feito:
		if ok || g.Descartados() != 1 {
			t.Fatalf("ok=%v descartados=%d", ok, g.Descartados())
		}
	case <-time.After(time.Second):
		t.Fatal("Registrar travou com o canal cheio")
	}
}

func TestRelatorio(t *testing.T) {
	b := abrir(t)
	loja, _ := b.CriarCliente(ctx, Cliente{Nome: "loja"})
	app, _ := b.CriarCliente(ctx, Cliente{Nome: "app"})
	base := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	var eventos []Evento
	for i := 1; i <= 100; i++ {
		e := Evento{TS: base.Add(time.Duration(i) * time.Second), ClienteID: loja.ID, Modelo: "rapido", Destino: "deepseek/deepseek-flash",
			Status: 200, LatenciaMs: int64(i), CustoMicro: 10, PII: 1, PromptTokens: 5, CompletionTokens: 5}
		if i%10 == 0 {
			e.Cache, e.CustoMicro, e.EconomiaMicro = true, 0, 10
		}
		eventos = append(eventos, e)
	}
	eventos = append(eventos,
		Evento{TS: base, ClienteID: app.ID, Modelo: "forte", Status: 503, LatenciaMs: 1},
		Evento{TS: base, ClienteID: loja.ID, Modelo: "migracao", Destino: "deepseek/deepseek-v4-pro", Status: 200, Shadow: true, LatenciaMs: 900, CustoMicro: 50, TamanhoResposta: 300},
	)
	if err := b.gravar(ctx, eventos); err != nil {
		t.Fatal(err)
	}
	r, err := b.Relatorio(ctx, base.Add(-time.Hour), base.Add(time.Hour), 0)
	if err != nil {
		t.Fatal(err)
	}
	tot := r.Total
	if tot.Requisicoes != 101 || tot.Erros != 1 || tot.CacheHits != 10 || tot.CustoMicro != 900 || tot.EconomiaMicro != 100 || tot.PII != 100 {
		t.Fatalf("Total = %+v (shadow não entra nos totais)", tot)
	}
	if tot.P95Ms != 95 {
		t.Errorf("P95Ms = %d, queria 95", tot.P95Ms)
	}
	if len(r.PorCliente) != 2 || r.PorCliente[0].Nome != "loja" {
		t.Errorf("PorCliente = %+v", r.PorCliente)
	}
	if len(r.Serie) != 2 {
		t.Errorf("Serie = %+v, queria 2 minutos", r.Serie)
	}
	if len(r.Shadow) != 1 || r.Shadow[0].Destino != "deepseek/deepseek-v4-pro" || r.Shadow[0].LatenciaMediaMs != 900 {
		t.Errorf("Shadow = %+v", r.Shadow)
	}
	if so, _ := b.Relatorio(ctx, base.Add(-time.Hour), base.Add(time.Hour), app.ID); so.Total.Requisicoes != 1 {
		t.Errorf("filtro por cliente: %+v", so.Total)
	}

	gasto, err := b.GastoDoMes(ctx, base)
	if err != nil || gasto[loja.ID] != 900 {
		t.Fatalf("GastoDoMes = %v, %v", gasto, err)
	}
}

// Um ID removido nunca volta: senão o cliente novo herdaria histórico de uso, gasto do mês
// e limitador do cliente removido.
func TestIDNuncaReaproveitado(t *testing.T) {
	b := abrir(t)
	b.CriarCliente(ctx, Cliente{Nome: "a"})
	velho, _ := b.CriarCliente(ctx, Cliente{Nome: "b"})
	_, kVelha, _ := b.CriarChave(ctx, velho.ID)
	if err := b.RemoverCliente(ctx, velho.ID); err != nil {
		t.Fatal(err)
	}
	novo, _ := b.CriarCliente(ctx, Cliente{Nome: "c"})
	if novo.ID == velho.ID {
		t.Fatalf("cliente novo reaproveitou o ID %d do removido", novo.ID)
	}
	_, kNova, _ := b.CriarChave(ctx, novo.ID)
	if kNova.ID == kVelha.ID {
		t.Fatalf("chave nova reaproveitou o ID %d", kNova.ID)
	}
}

// A demo pública sobe com banco vazio (disco efêmero): o seed cria o cliente demo com a
// chave publicada no README. Com clientes já cadastrados, não mexe em nada.
func TestSemearDemo(t *testing.T) {
	b := abrir(t)
	const chave = "ck_demo_publica"
	criou, err := b.SemearDemo(ctx, chave)
	if err != nil || !criou {
		t.Fatalf("primeiro seed: criou=%v err=%v", criou, err)
	}
	ativas, _ := b.ChavesAtivas(ctx)
	clientes, _ := b.ListarClientes(ctx)
	if len(clientes) != 1 || !clientes[0].LGPD || ativas[HashChave(chave)] != clientes[0].ID {
		t.Fatalf("clientes=%+v ativas=%v", clientes, ativas)
	}
	if criou, err := b.SemearDemo(ctx, chave); err != nil || criou {
		t.Fatalf("segundo seed não pode criar de novo: criou=%v err=%v", criou, err)
	}
	if _, err := b.SemearDemo(ctx, "sem_prefixo"); err == nil {
		t.Fatal("chave sem ck_ seria recusada pela autenticação: o seed tem de recusar também")
	}
}
