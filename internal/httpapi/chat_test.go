package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/venturelli-91/comporta_proxy/internal/armazenamento"
	"github.com/venturelli-91/comporta_proxy/internal/cache"
	"github.com/venturelli-91/comporta_proxy/internal/gateway"
	"github.com/venturelli-91/comporta_proxy/internal/provedor"
)

const tokenAdmin = "admin-teste"

// espiao guarda o último corpo que o provedor recebeu, para provar o que saiu do gateway.
type espiao struct {
	mu     sync.Mutex
	ultimo string
	h      http.Handler
}

func (e *espiao) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	e.mu.Lock()
	e.ultimo = string(b)
	e.mu.Unlock()
	r.Body = io.NopCloser(bytes.NewReader(b))
	e.h.ServeHTTP(w, r)
}

func (e *espiao) corpo() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ultimo
}

type cenario struct {
	t        testing.TB
	s        *Servidor
	url      string
	http     *httptest.Server
	falso    *provedor.Falso
	espiao   *espiao
	banco    *armazenamento.Banco
	pararUso func()
}

// novoCenario: provedor "falso" (rápido), "ruim" (sempre 503) e "lento" (só keep-alive).
// Aliases: rapido → falso/m1; fallback → [ruim/m1, falso/m2]; migracao → falso/m1 com shadow falso/m2;
// travado → lento/m1.
func novoCenario(t testing.TB) *cenario {
	t.Helper()
	c := &cenario{t: t, falso: &provedor.Falso{}}
	c.espiao = &espiao{h: c.falso}
	lento := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for {
			fmt.Fprint(w, ": keep-alive\n\n")
			_ = http.NewResponseController(w).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	})
	// mudo aceita a conexão e nunca manda cabeçalho.
	// Lê o corpo antes de esperar: só assim o servidor HTTP/1.1 percebe a desconexão do cliente.
	mudo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	})
	servidores := map[string]http.Handler{"falso": c.espiao, "ruim": &provedor.Falso{TaxaErro: 1}, "lento": lento, "mudo": mudo}
	clientes := map[string]*provedor.Cliente{}
	provs := map[string]gateway.ProvedorCfg{}
	for nome, h := range servidores {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		clientes[nome] = provedor.Novo(srv.URL, "", 50, time.Second)
		provs[nome] = gateway.ProvedorCfg{}
	}
	cfg := &gateway.Config{
		Provedores: provs,
		Modelos: map[string]gateway.ModeloCfg{
			"rapido":   {Destinos: []gateway.Destino{{Provedor: "falso", Modelo: "m1"}}, Concorrencia: 100},
			"fallback": {Destinos: []gateway.Destino{{Provedor: "ruim", Modelo: "m1"}, {Provedor: "falso", Modelo: "m2"}}, Concorrencia: 100},
			"migracao": {Destinos: []gateway.Destino{{Provedor: "falso", Modelo: "m1"}}, Shadow: &gateway.Destino{Provedor: "falso", Modelo: "m2"}, Concorrencia: 100},
			"travado":  {Destinos: []gateway.Destino{{Provedor: "lento", Modelo: "m1"}}, Concorrencia: 100},
			"calado":   {Destinos: []gateway.Destino{{Provedor: "mudo", Modelo: "m1"}}, Concorrencia: 100},
		},
		Precos: map[string]gateway.PrecoCfg{"m1": {EntradaCacheMiss: 100, Saida: 1000}, "m2": {EntradaCacheMiss: 200, Saida: 2000}},
	}
	b, err := armazenamento.Abrir(filepath.Join(t.TempDir(), "teste.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Fechar() })
	c.banco = b
	g := armazenamento.NovoGravador(b, 1000)
	ctx, parar := context.WithCancel(context.Background())
	fim := make(chan struct{})
	go func() { g.Rodar(ctx); close(fim) }()
	var uma sync.Once
	c.pararUso = func() { uma.Do(func() { parar(); <-fim }) }
	t.Cleanup(c.pararUso)

	c.s, err = Novo(Deps{
		Config: cfg, Roteador: gateway.NovoRoteador(cfg, clientes), Banco: b, Gravador: g,
		Cache: cache.Novo(1<<20, time.Minute), AdminToken: tokenAdmin,
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		ConfiarProxy: true, // os testes simulam o Cloud Run: o XFF separa os IPs
	})
	if err != nil {
		t.Fatal(err)
	}
	c.http = httptest.NewServer(c.s.Handler())
	t.Cleanup(c.http.Close)
	c.url = c.http.URL
	return c
}

func (c *cenario) admin(metodo, caminho string, corpo any) *http.Response {
	c.t.Helper()
	b, _ := json.Marshal(corpo)
	req, _ := http.NewRequest(metodo, c.url+caminho, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+tokenAdmin)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp
}

// novoCliente cria um cliente pelo admin e devolve o id e a chave ck_.
func (c *cenario) novoCliente(cli armazenamento.Cliente) (int64, string) {
	c.t.Helper()
	resp := c.admin("POST", "/admin/clientes", cli)
	var criado armazenamento.Cliente
	_ = json.NewDecoder(resp.Body).Decode(&criado)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		c.t.Fatalf("criar cliente: %d", resp.StatusCode)
	}
	resp = c.admin("POST", fmt.Sprintf("/admin/clientes/%d/chaves", criado.ID), nil)
	var k struct{ Chave string }
	_ = json.NewDecoder(resp.Body).Decode(&k)
	resp.Body.Close()
	return criado.ID, k.Chave
}

func (c *cenario) chat(chave, corpo string, headers ...string) *http.Response {
	c.t.Helper()
	return c.chatCtx(context.Background(), chave, corpo, headers...)
}

func (c *cenario) chatCtx(ctx context.Context, chave, corpo string, headers ...string) *http.Response {
	c.t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "POST", c.url+"/v1/chat/completions", strings.NewReader(corpo))
	if chave != "" {
		req.Header.Set("Authorization", "Bearer "+chave)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp
}

func msg(modelo, texto string, stream bool) string {
	b, _ := json.Marshal(map[string]any{"model": modelo, "stream": stream,
		"messages": []map[string]string{{"role": "user", "content": texto}}})
	return string(b)
}

// conteudo devolve o texto da resposta, juntando os deltas quando é stream.
func conteudo(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		var v struct {
			Choices []struct {
				Message struct{ Content string } `json:"message"`
			} `json:"choices"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&v)
		return v.Choices[0].Message.Content
	}
	var out strings.Builder
	sc := bufio.NewScanner(resp.Body)
	fim := false
	for sc.Scan() {
		l := sc.Text()
		if l == "data: [DONE]" {
			fim = true
			continue
		}
		if !strings.HasPrefix(l, "data: ") {
			continue
		}
		var v struct {
			Choices []struct {
				Delta struct{ Content string } `json:"delta"`
			} `json:"choices"`
		}
		_ = json.Unmarshal([]byte(l[6:]), &v)
		for _, ch := range v.Choices {
			out.WriteString(ch.Delta.Content)
		}
	}
	if !fim {
		t.Fatal("stream sem [DONE]")
	}
	return out.String()
}

func erroCodigo(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var v struct {
		Erro struct{ Codigo, Mensagem string } `json:"erro"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("corpo de erro fora do formato: %v", err)
	}
	return v.Erro.Codigo
}

// relatorio fecha o servidor (Close espera os handlers em andamento), espera os shadows,
// drena o gravador e lê o uso registrado. Depois dele o cenário não aceita mais requisições.
func (c *cenario) relatorio() armazenamento.Relatorio {
	c.t.Helper()
	c.http.Close()
	c.s.Esperar()
	c.pararUso()
	r, err := c.banco.Relatorio(context.Background(), time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 0)
	if err != nil {
		c.t.Fatal(err)
	}
	return r
}

func TestChaveInvalida(t *testing.T) {
	c := novoCenario(t)
	for _, chave := range []string{"", "ck_naoexiste"} {
		resp := c.chat(chave, msg("rapido", "oi", false))
		if resp.StatusCode != 401 || erroCodigo(t, resp) != "chave_invalida" {
			t.Errorf("chave %q: status %d", chave, resp.StatusCode)
		}
	}
}

func TestSemStream(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	if got := conteudo(t, c.chat(chave, msg("rapido", "olá", false))); got != "Resposta do m1: olá" {
		t.Fatalf("conteúdo = %q", got)
	}
	r := c.relatorio()
	if r.Total.Requisicoes != 1 || r.Total.CustoMicro == 0 || r.Total.Tokens == 0 {
		t.Fatalf("uso registrado = %+v", r.Total)
	}
}

func TestStream(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	if got := conteudo(t, c.chat(chave, msg("rapido", "olá mundo", true))); got != "Resposta do m1: olá mundo" {
		t.Fatalf("conteúdo = %q", got)
	}
	if !strings.Contains(c.espiao.corpo(), `"include_usage":true`) {
		t.Error("o gateway devia pedir include_usage para medir o custo do stream")
	}
	if r := c.relatorio(); r.Total.CustoMicro == 0 {
		t.Fatalf("stream sem custo registrado: %+v", r.Total)
	}
}

func TestRateLimit(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja", RPM: 1})
	conteudo(t, c.chat(chave, msg("rapido", "oi", false)))
	resp := c.chat(chave, msg("rapido", "oi", false))
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") == "" || erroCodigo(t, resp) != "limite_excedido" {
		t.Fatalf("status %d, Retry-After %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
}

func TestOrcamento(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja", OrcamentoMicro: 1})
	conteudo(t, c.chat(chave, msg("rapido", "oi", false)))
	resp := c.chat(chave, msg("rapido", "oi", false))
	if resp.StatusCode != 402 || erroCodigo(t, resp) != "orcamento_esgotado" {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestModeloNaoLiberado(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja", Modelos: []string{"rapido"}})
	if resp := c.chat(chave, msg("fallback", "oi", false)); resp.StatusCode != 403 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if resp := c.chat(chave, msg("inexistente", "oi", false)); resp.StatusCode != 400 {
		t.Fatalf("modelo desconhecido: status %d", resp.StatusCode)
	}
}

func TestCorpoGrande(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	resp := c.chat(chave, msg("rapido", strings.Repeat("a", 2<<20), false))
	if resp.StatusCode != 413 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestCacheHit(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	for range 2 {
		conteudo(t, c.chat(chave, msg("rapido", "pergunta", false), "X-Comporta-Cache", "ligado"))
	}
	if n := c.falso.Chamadas.Load(); n != 1 {
		t.Fatalf("provedor chamado %d vezes, queria 1", n)
	}
	if r := c.relatorio(); r.Total.CacheHits != 1 || r.Total.EconomiaMicro == 0 {
		t.Fatalf("uso = %+v", r.Total)
	}
}

func TestSemCacheSemHeader(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	for range 2 {
		conteudo(t, c.chat(chave, msg("rapido", "pergunta", false)))
	}
	if n := c.falso.Chamadas.Load(); n != 2 {
		t.Fatalf("cache é opt-in: provedor chamado %d vezes, queria 2", n)
	}
}

func TestSingleflight(t *testing.T) {
	c := novoCenario(t)
	c.falso.Latencia = 200 * time.Millisecond
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja", Cache: true})
	var wg sync.WaitGroup
	respostas := make([]string, 10)
	for i := range respostas {
		wg.Go(func() { respostas[i] = conteudo(t, c.chat(chave, msg("rapido", "igual", true))) })
	}
	wg.Wait()
	if n := c.falso.Chamadas.Load(); n != 1 {
		t.Fatalf("10 pedidos iguais simultâneos viraram %d chamadas, queria 1", n)
	}
	for _, r := range respostas {
		if r != "Resposta do m1: igual" {
			t.Fatalf("resposta = %q", r)
		}
	}
}

func TestLGPDIdaEVolta(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint("stream=", stream), func(t *testing.T) {
			c := novoCenario(t)
			_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja", LGPD: true})
			got := conteudo(t, c.chat(chave, msg("rapido", "meu CPF é 529.982.247-25 e o e-mail a@b.com", stream)))
			if got != "Resposta do m1: meu CPF é 529.982.247-25 e o e-mail a@b.com" {
				t.Fatalf("cliente recebeu %q", got)
			}
			enviado := c.espiao.corpo()
			if strings.Contains(enviado, "529.982.247-25") || strings.Contains(enviado, "a@b.com") || !strings.Contains(enviado, "[CPF_1]") {
				t.Fatalf("provedor recebeu dado pessoal: %s", enviado)
			}
			if r := c.relatorio(); r.Total.PII != 2 {
				t.Fatalf("PII = %d, queria 2", r.Total.PII)
			}
		})
	}
}

func TestFallback(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	if got := conteudo(t, c.chat(chave, msg("fallback", "oi", false))); got != "Resposta do m2: oi" {
		t.Fatalf("conteúdo = %q (devia cair no destino reserva)", got)
	}
}

func TestRevogacaoValeNaHora(t *testing.T) {
	c := novoCenario(t)
	id, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	conteudo(t, c.chat(chave, msg("rapido", "oi", false)))
	resp := c.admin("GET", fmt.Sprintf("/admin/clientes/%d/chaves", id), nil)
	var chaves []armazenamento.Chave
	_ = json.NewDecoder(resp.Body).Decode(&chaves)
	resp.Body.Close()
	resp = c.admin("DELETE", fmt.Sprintf("/admin/clientes/%d/chaves/%d", id, chaves[0].ID), nil)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("revogar: %d", resp.StatusCode)
	}
	if resp := c.chat(chave, msg("rapido", "oi", false)); resp.StatusCode != 401 {
		t.Fatalf("chave revogada ainda funciona: %d", resp.StatusCode)
	}
}

func TestShadow(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	if got := conteudo(t, c.chat(chave, msg("migracao", "oi", true))); got != "Resposta do m1: oi" {
		t.Fatalf("o cliente só recebe a resposta do principal; veio %q", got)
	}
	r := c.relatorio()
	if r.Total.Requisicoes != 1 {
		t.Errorf("shadow não conta como requisição: %+v", r.Total)
	}
	if len(r.Shadow) != 2 || !r.Shadow[1].Shadow || r.Shadow[1].Destino != "falso/m2" {
		t.Fatalf("comparação = %+v", r.Shadow)
	}
}

func TestClienteDesconectaNoMeioDoStream(t *testing.T) {
	c := novoCenario(t)
	c.falso.PorToken = 50 * time.Millisecond
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	ctx, cancel := context.WithCancel(context.Background())
	resp := c.chatCtx(ctx, chave, msg("rapido", strings.Repeat("palavra ", 50), true))
	_, _ = bufio.NewReader(resp.Body).ReadString('\n')
	cancel()
	resp.Body.Close()
	r := c.relatorio()
	if r.Total.Requisicoes != 1 || r.Total.Erros != 1 {
		t.Fatalf("desconexão devia virar um evento com status 499: %+v", r.Total)
	}
}

func TestWatchdogSoKeepAlive(t *testing.T) {
	c := novoCenario(t)
	c.s.primeiroToken = 100 * time.Millisecond
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	inicio := time.Now()
	resp := c.chat(chave, msg("travado", "oi", true))
	if resp.StatusCode != 504 || erroCodigo(t, resp) != "timeout" {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if time.Since(inicio) > 2*time.Second {
		t.Fatal("keep-alive não pode contar como progresso")
	}
}

func TestModelosEMetricas(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja", Modelos: []string{"rapido"}})
	req, _ := http.NewRequest("GET", c.url+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+chave)
	resp, _ := http.DefaultClient.Do(req)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `"id":"rapido"`) || strings.Contains(string(b), "fallback") {
		t.Fatalf("/v1/models = %s", b)
	}
	resp, _ = http.Get(c.url + "/metrics")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "comporta_requisicoes_total") {
		t.Fatalf("/metrics = %s", b)
	}
	if resp, _ := http.Get(c.url + "/admin/clientes"); resp.StatusCode != 401 {
		t.Fatalf("admin sem token: %d", resp.StatusCode)
	}
}

func TestWatchdogAntesDosCabecalhos(t *testing.T) {
	c := novoCenario(t)
	c.s.primeiroToken = 100 * time.Millisecond
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	inicio := time.Now()
	resp := c.chat(chave, msg("calado", "oi", true))
	if resp.StatusCode != 504 || erroCodigo(t, resp) != "timeout" || time.Since(inicio) > 2*time.Second {
		t.Fatalf("provedor que nunca manda cabeçalho: status %d em %v", resp.StatusCode, time.Since(inicio))
	}
}

func TestWatchdogNoCaminhoDoCache(t *testing.T) {
	c := novoCenario(t)
	c.s.primeiroToken = 100 * time.Millisecond
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja", Cache: true})
	inicio := time.Now()
	resp := c.chat(chave, msg("travado", "oi", true))
	if resp.StatusCode != 504 || time.Since(inicio) > 2*time.Second {
		t.Fatalf("cache + só keep-alive: status %d em %v", resp.StatusCode, time.Since(inicio))
	}
}

// Sem stream_options pedido pelo cliente, o chunk de usage com choices vazio não pode chegar
// a ele: o laço clássico chunk.choices[0] dos SDKs quebraria.
func TestUsageSoQuandoOClientePede(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	ler := func(corpo string) string {
		resp := c.chat(chave, corpo)
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if b := ler(msg("rapido", "oi", true)); strings.Contains(b, `"choices":[]`) {
		t.Fatalf("cliente não pediu usage e recebeu chunk sem choices:\n%s", b)
	}
	pediu := `{"model":"rapido","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"oi"}]}`
	if b := ler(pediu); !strings.Contains(b, `"usage"`) {
		t.Fatalf("cliente pediu usage e não recebeu:\n%s", b)
	}
}

func TestStreamCortadoAindaCobra(t *testing.T) {
	c := novoCenario(t)
	c.falso.PorToken = 50 * time.Millisecond
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	ctx, cancel := context.WithCancel(context.Background())
	resp := c.chatCtx(ctx, chave, msg("rapido", strings.Repeat("palavra ", 50), true))
	br := bufio.NewReader(resp.Body)
	for range 6 {
		_, _ = br.ReadString('\n')
	}
	cancel()
	resp.Body.Close()
	if r := c.relatorio(); r.Total.CustoMicro == 0 {
		t.Fatalf("stream cortado foi registrado sem custo: %+v", r.Total)
	}
}

func TestShadowLimitado(t *testing.T) {
	c := novoCenario(t)
	c.s.sombras = make(chan struct{}) // sem vaga nenhuma
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	conteudo(t, c.chat(chave, msg("migracao", "oi", false)))
	if n := c.falso.Chamadas.Load(); n != 1 {
		t.Fatalf("shadow sem vaga devia ser descartado: provedor chamado %d vezes", n)
	}
	if c.s.met.sombrasDescartadas.Load() != 1 {
		t.Fatal("descarte de shadow não foi contado")
	}
}

// Um cliente não pode ocupar todas as vagas do modelo: passa do limite próprio e leva 429,
// enquanto os outros clientes continuam atendidos.
func TestLimiteSimultaneoPorCliente(t *testing.T) {
	c := novoCenario(t)
	c.s.porCliente = 1
	c.falso.PorToken = 30 * time.Millisecond
	_, guloso := c.novoCliente(armazenamento.Cliente{Nome: "guloso"})
	_, outro := c.novoCliente(armazenamento.Cliente{Nome: "outro"})
	resp := c.chat(guloso, msg("rapido", strings.Repeat("palavra ", 30), true))
	defer resp.Body.Close()
	_, _ = bufio.NewReader(resp.Body).ReadString('\n') // stream aberto, vaga ocupada

	segundo := c.chat(guloso, msg("rapido", "oi", false))
	if segundo.StatusCode != 429 || segundo.Header.Get("Retry-After") == "" || erroCodigo(t, segundo) != "concorrencia_excedida" {
		t.Fatalf("segunda requisição simultânea do mesmo cliente: status %d", segundo.StatusCode)
	}
	if got := conteudo(t, c.chat(outro, msg("rapido", "oi", false))); got != "Resposta do m1: oi" {
		t.Fatalf("outro cliente foi afetado: %q", got)
	}
}

func (c *cenario) login(token string, headers ...string) *http.Response {
	c.t.Helper()
	req, _ := http.NewRequest("POST", c.url+"/admin/login", strings.NewReader(`{"token":"`+token+`"}`))
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func (c *cenario) comCookie(metodo, caminho string, ck *http.Cookie) int {
	c.t.Helper()
	req, _ := http.NewRequest(metodo, c.url+caminho, nil)
	req.AddCookie(ck)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// Um cookie copiado não pode continuar valendo depois do logout.
func TestLogoutRevogaSessao(t *testing.T) {
	c := novoCenario(t)
	ck := c.login(tokenAdmin).Cookies()[0]
	if st := c.comCookie("GET", "/admin/sessao", ck); st != 204 {
		t.Fatalf("sessão recém-criada: %d", st)
	}
	c.comCookie("POST", "/admin/logout", ck)
	if st := c.comCookie("GET", "/admin/sessao", ck); st != 401 {
		t.Fatalf("cookie depois do logout ainda vale: %d", st)
	}
}

// O limite de login é por IP: o login certo também consome, mas o atacante esgota só o
// próprio limite e não tranca o admin de verdade, que entra de outro IP.
func TestLoginLimitaTentativasSemTrancarOAdmin(t *testing.T) {
	c := novoCenario(t)
	var ultimo int
	for range 6 {
		ultimo = c.login("chute", "X-Forwarded-For", "203.0.113.9").StatusCode
	}
	if ultimo != 429 {
		t.Fatalf("6ª tentativa errada do mesmo IP: status %d, queria 429", ultimo)
	}
	if st := c.login(tokenAdmin, "X-Forwarded-For", "198.51.100.7").StatusCode; st != 204 {
		t.Fatalf("admin em outro IP foi trancado pelo atacante: %d", st)
	}
}

// 20 tentativas erradas ao mesmo tempo do mesmo IP: no máximo 5 podem chegar à comparação.
// Com checar-e-depois-consumir, todas passavam pela checagem antes de alguma consumir.
func TestLoginParaleloRespeitaLimite(t *testing.T) {
	c := novoCenario(t)
	var wg sync.WaitGroup
	var comparadas atomic.Int32
	for range 20 {
		wg.Go(func() {
			req, _ := http.NewRequest("POST", c.url+"/admin/login", strings.NewReader(`{"token":"chute"}`))
			req.Header.Set("X-Forwarded-For", "203.0.113.9")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err) // t.Fatal não pode ser chamado fora da goroutine do teste
				return
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized {
				comparadas.Add(1)
			}
		})
	}
	wg.Wait()
	if n := comparadas.Load(); n > 5 {
		t.Fatalf("%d tentativas chegaram à comparação; o limite é 5", n)
	}
}

// Atrás do TLS do Cloud Run a requisição chega em HTTP puro; o cookie ainda precisa de Secure.
func TestCookieSecureAtrasDeProxy(t *testing.T) {
	c := novoCenario(t)
	ck := c.login(tokenAdmin, "X-Forwarded-Proto", "https").Cookies()[0]
	if !ck.Secure || !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %+v", ck)
	}
}

// Sem proxy confiável na frente, o XFF é do cliente: trocá-lo não pode render um balde novo.
func TestIPDe(t *testing.T) {
	casos := []struct {
		nome    string
		confiar bool
		xff     string
		quer    string
	}{
		{"sem proxy ignora xff", false, "1.2.3.4, 203.0.113.9", "192.0.2.1"}, // RemoteAddr do httptest
		{"cloud run usa a última", true, "1.2.3.4,  203.0.113.9", "203.0.113.9"},
		{"cloud run entrada única", true, "203.0.113.9", "203.0.113.9"},
		{"cloud run sem xff", true, "", "192.0.2.1"},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			s := &Servidor{d: Deps{ConfiarProxy: tc.confiar}}
			r := httptest.NewRequest("POST", "/admin/login", nil)
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := s.ipDe(r); got != tc.quer {
				t.Fatalf("ipDe = %q, queria %q", got, tc.quer)
			}
		})
	}
}
