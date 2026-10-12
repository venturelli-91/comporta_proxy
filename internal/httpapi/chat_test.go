package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
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

// espiao (spy) keeps the last body the provider received, to prove what left the gateway.
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
	dbPath   string // for tests that read the usage table directly
	pararUso func()
}

// novoCenario: providers "falso" (fast fake), "ruim" (always 503) and "lento" (keep-alive only).
// Aliases: rapido → falso/m1; fallback → [ruim/m1, falso/m2]; migracao → falso/m1 with shadow falso/m2;
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
	// mudo (mute) accepts the connection and never sends a header.
	// It reads the body before waiting: only then does the HTTP/1.1 server notice the client disconnecting.
	mudo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	})
	// gigante (giant) answers with a body 1 byte above the read ceiling.
	gigante := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bytes.Repeat([]byte("a"), maxResposta+1))
	})
	// pensador (thinker) sends reasoning with the [CPF_1] placeholder split across two chunks, like DeepSeek.
	pensador := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, d := range []string{`{"reasoning_content":"o CPF é [CP"}`, `{"reasoning_content":"F_1] mesmo ["}`, `{"content":"ok"}`} {
			fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":%s}]}\n\n", d)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	servidores := map[string]http.Handler{"pensador": pensador, "gigante": gigante, "falso": c.espiao, "ruim": &provedor.Falso{TaxaErro: 1}, "lento": lento, "mudo": mudo}
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
			"enorme":   {Destinos: []gateway.Destino{{Provedor: "gigante", Modelo: "m1"}}, Concorrencia: 100},
			"pensa":    {Destinos: []gateway.Destino{{Provedor: "pensador", Modelo: "m1"}}, Concorrencia: 100},
			"presa":    {Destinos: []gateway.Destino{{Provedor: "falso", Modelo: "m1"}}, Shadow: &gateway.Destino{Provedor: "mudo", Modelo: "m2"}, Concorrencia: 100},
		},
		Precos:   map[string]gateway.PrecoCfg{"m1": {EntradaCacheMiss: 100, Saida: 1000}, "m2": {EntradaCacheMiss: 200, Saida: 2000}},
		Timeouts: gateway.TimeoutsCfg{PrimeiroTokenS: 30, EntreChunksS: 60, TotalS: 600}, // no Carregar, so no defaults
	}
	c.dbPath = filepath.Join(t.TempDir(), "teste.db")
	b, err := armazenamento.Abrir(c.dbPath)
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
		ConfiarProxy: true, // the tests simulate Cloud Run: XFF separates the IPs
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

// novoCliente creates a client through the admin and returns its id and ck_ key.
func (c *cenario) novoCliente(cli armazenamento.Cliente) (int64, string) {
	c.t.Helper()
	resp := c.admin("POST", "/admin/clientes", cli)
	var criado armazenamento.Cliente
	_ = json.NewDecoder(resp.Body).Decode(&criado)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		c.t.Fatalf("create client: %d", resp.StatusCode)
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

// conteudo returns the response text, joining the deltas when streaming.
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
		t.Fatal("stream without [DONE]")
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
		t.Fatalf("error body in the wrong format: %v", err)
	}
	return v.Erro.Codigo
}

// relatorio closes the server (Close waits for in-flight handlers), waits for the shadows,
// drains the writer and reads the recorded usage. After it the scenario accepts no more requests.
func (c *cenario) relatorio() armazenamento.Relatorio {
	c.t.Helper()
	c.http.Close()
	c.s.Encerrar(context.Background())
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
			t.Errorf("key %q: status %d", chave, resp.StatusCode)
		}
	}
}

func TestSemStream(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	if got := conteudo(t, c.chat(chave, msg("rapido", "olá", false))); got != "Answer from m1: olá" {
		t.Fatalf("content = %q", got)
	}
	r := c.relatorio()
	if r.Total.Requisicoes != 1 || r.Total.CustoMicro == 0 || r.Total.Tokens == 0 {
		t.Fatalf("recorded usage = %+v", r.Total)
	}
}

func TestStream(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	if got := conteudo(t, c.chat(chave, msg("rapido", "olá mundo", true))); got != "Answer from m1: olá mundo" {
		t.Fatalf("content = %q", got)
	}
	if !strings.Contains(c.espiao.corpo(), `"include_usage":true`) {
		t.Error("the gateway should ask for include_usage to measure the stream's cost")
	}
	if r := c.relatorio(); r.Total.CustoMicro == 0 {
		t.Fatalf("stream without a recorded cost: %+v", r.Total)
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
		t.Fatalf("unknown model: status %d", resp.StatusCode)
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
		t.Fatalf("provider called %d times, want 1", n)
	}
	if r := c.relatorio(); r.Total.CacheHits != 1 || r.Total.EconomiaMicro == 0 {
		t.Fatalf("usage = %+v", r.Total)
	}
}

func TestSemCacheSemHeader(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	for range 2 {
		conteudo(t, c.chat(chave, msg("rapido", "pergunta", false)))
	}
	if n := c.falso.Chamadas.Load(); n != 2 {
		t.Fatalf("cache is opt-in: provider called %d times, want 2", n)
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
		t.Fatalf("10 identical concurrent requests became %d calls, want 1", n)
	}
	for _, r := range respostas {
		if r != "Answer from m1: igual" {
			t.Fatalf("response = %q", r)
		}
	}
}

func TestLGPDIdaEVolta(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint("stream=", stream), func(t *testing.T) {
			c := novoCenario(t)
			_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja", LGPD: true})
			got := conteudo(t, c.chat(chave, msg("rapido", "meu CPF é 529.982.247-25 e o e-mail a@b.com", stream)))
			if got != "Answer from m1: meu CPF é 529.982.247-25 e o e-mail a@b.com" {
				t.Fatalf("client received %q", got)
			}
			enviado := c.espiao.corpo()
			if strings.Contains(enviado, "529.982.247-25") || strings.Contains(enviado, "a@b.com") || !strings.Contains(enviado, "[CPF_1]") {
				t.Fatalf("provider received personal data: %s", enviado)
			}
			if r := c.relatorio(); r.Total.PII != 2 {
				t.Fatalf("PII = %d, want 2", r.Total.PII)
			}
		})
	}
}

func TestFallback(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	if got := conteudo(t, c.chat(chave, msg("fallback", "oi", false))); got != "Answer from m2: oi" {
		t.Fatalf("content = %q (should fall back to the backup destination)", got)
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
		t.Fatalf("revoke: %d", resp.StatusCode)
	}
	if resp := c.chat(chave, msg("rapido", "oi", false)); resp.StatusCode != 401 {
		t.Fatalf("revoked key still works: %d", resp.StatusCode)
	}
}

func TestShadow(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	if got := conteudo(t, c.chat(chave, msg("migracao", "oi", true))); got != "Answer from m1: oi" {
		t.Fatalf("the client only gets the primary's response; got %q", got)
	}
	r := c.relatorio()
	if r.Total.Requisicoes != 1 {
		t.Errorf("shadow does not count as a request: %+v", r.Total)
	}
	if len(r.Shadow) != 2 || !r.Shadow[1].Shadow || r.Shadow[1].Destino != "falso/m2" {
		t.Fatalf("comparison = %+v", r.Shadow)
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
		t.Fatalf("a disconnect should become an event with status 499: %+v", r.Total)
	}
	// Erros counts any status >= 400: check that it's the 499 (client gave up), not a 502.
	db, err := sql.Open("sqlite", c.dbPath) // driver registered by the armazenamento package
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var status int
	if err := db.QueryRow(`SELECT status FROM uso WHERE shadow = 0`).Scan(&status); err != nil || status != 499 {
		t.Fatalf("recorded status = %d (err %v), want 499", status, err)
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
		t.Fatal("keep-alive must not count as progress")
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
		t.Fatalf("admin without token: %d", resp.StatusCode)
	}
}

func TestWatchdogAntesDosCabecalhos(t *testing.T) {
	c := novoCenario(t)
	c.s.primeiroToken = 100 * time.Millisecond
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	inicio := time.Now()
	resp := c.chat(chave, msg("calado", "oi", true))
	if resp.StatusCode != 504 || erroCodigo(t, resp) != "timeout" || time.Since(inicio) > 2*time.Second {
		t.Fatalf("provider that never sends a header: status %d in %v", resp.StatusCode, time.Since(inicio))
	}
}

func TestWatchdogNoCaminhoDoCache(t *testing.T) {
	c := novoCenario(t)
	c.s.primeiroToken = 100 * time.Millisecond
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja", Cache: true})
	inicio := time.Now()
	resp := c.chat(chave, msg("travado", "oi", true))
	if resp.StatusCode != 504 || time.Since(inicio) > 2*time.Second {
		t.Fatalf("cache + keep-alive only: status %d in %v", resp.StatusCode, time.Since(inicio))
	}
}

// Unless the client asked for stream_options, the usage chunk with empty choices must not reach
// it: the SDKs' classic chunk.choices[0] loop would break.
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
		t.Fatalf("client did not ask for usage and got a chunk without choices:\n%s", b)
	}
	pediu := `{"model":"rapido","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"oi"}]}`
	if b := ler(pediu); !strings.Contains(b, `"usage"`) {
		t.Fatalf("client asked for usage and did not get it:\n%s", b)
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
		t.Fatalf("cut stream was recorded without cost: %+v", r.Total)
	}
}

func TestShadowLimitado(t *testing.T) {
	c := novoCenario(t)
	c.s.sombras = make(chan struct{}) // no free slots at all
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	conteudo(t, c.chat(chave, msg("migracao", "oi", false)))
	if n := c.falso.Chamadas.Load(); n != 1 {
		t.Fatalf("a shadow without a slot should be dropped: provider called %d times", n)
	}
	if c.s.met.sombrasDescartadas.Load() != 1 {
		t.Fatal("shadow drop was not counted")
	}
}

// One client can't take every slot of the model: past its own limit it gets a 429,
// while the other clients keep being served.
func TestLimiteSimultaneoPorCliente(t *testing.T) {
	c := novoCenario(t)
	c.s.porCliente = 1
	c.falso.PorToken = 30 * time.Millisecond
	_, guloso := c.novoCliente(armazenamento.Cliente{Nome: "guloso"})
	_, outro := c.novoCliente(armazenamento.Cliente{Nome: "outro"})
	resp := c.chat(guloso, msg("rapido", strings.Repeat("palavra ", 30), true))
	defer resp.Body.Close()
	_, _ = bufio.NewReader(resp.Body).ReadString('\n') // stream open, slot taken

	segundo := c.chat(guloso, msg("rapido", "oi", false))
	if segundo.StatusCode != 429 || segundo.Header.Get("Retry-After") == "" || erroCodigo(t, segundo) != "concorrencia_excedida" {
		t.Fatalf("second concurrent request from the same client: status %d", segundo.StatusCode)
	}
	if got := conteudo(t, c.chat(outro, msg("rapido", "oi", false))); got != "Answer from m1: oi" {
		t.Fatalf("another client was affected: %q", got)
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

// A copied cookie can't keep working after logout.
func TestLogoutRevogaSessao(t *testing.T) {
	c := novoCenario(t)
	ck := c.login(tokenAdmin).Cookies()[0]
	if st := c.comCookie("GET", "/admin/sessao", ck); st != 204 {
		t.Fatalf("freshly created session: %d", st)
	}
	c.comCookie("POST", "/admin/logout", ck)
	if st := c.comCookie("GET", "/admin/sessao", ck); st != 401 {
		t.Fatalf("cookie still valid after logout: %d", st)
	}
}

// The login limit is per IP: a correct login also consumes, but the attacker only exhausts
// their own limit and doesn't lock out the real admin, who comes in from another IP.
func TestLoginLimitaTentativasSemTrancarOAdmin(t *testing.T) {
	c := novoCenario(t)
	var ultimo int
	for range 6 {
		ultimo = c.login("chute", "X-Forwarded-For", "203.0.113.9").StatusCode
	}
	if ultimo != 429 {
		t.Fatalf("6th wrong attempt from the same IP: status %d, want 429", ultimo)
	}
	if st := c.login(tokenAdmin, "X-Forwarded-For", "198.51.100.7").StatusCode; st != 204 {
		t.Fatalf("admin on another IP was locked out by the attacker: %d", st)
	}
}

// 20 wrong attempts at the same time from the same IP: at most 5 may reach the comparison.
// With check-then-consume, all of them passed the check before any consumed.
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
				t.Error(err) // t.Fatal can't be called outside the test goroutine
				return
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized {
				comparadas.Add(1)
			}
		})
	}
	wg.Wait()
	// Exactly 5: fewer would mean the limit consumes too much (or 429 for everyone), more would be the race.
	if n := comparadas.Load(); n != 5 {
		t.Fatalf("%d attempts reached the comparison; the limit is 5", n)
	}
}

// Behind Cloud Run's TLS the request arrives as plain HTTP; the cookie still needs Secure.
func TestCookieSecureAtrasDeProxy(t *testing.T) {
	c := novoCenario(t)
	ck := c.login(tokenAdmin, "X-Forwarded-Proto", "https").Cookies()[0]
	if !ck.Secure || !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %+v", ck)
	}
}

// Without a trusted proxy in front, XFF belongs to the client: changing it can't buy a fresh bucket.
func TestIPDe(t *testing.T) {
	casos := []struct {
		nome    string
		confiar bool
		xff     string
		quer    string
	}{
		{"no proxy ignores xff", false, "1.2.3.4, 203.0.113.9", "192.0.2.1"}, // httptest's RemoteAddr
		{"cloud run uses the last", true, "1.2.3.4,  203.0.113.9", "203.0.113.9"},
		{"cloud run single entry", true, "203.0.113.9", "203.0.113.9"},
		{"cloud run without xff", true, "", "192.0.2.1"},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			s := &Servidor{d: Deps{ConfiarProxy: tc.confiar}}
			r := httptest.NewRequest("POST", "/admin/login", nil)
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := s.ipDe(r); got != tc.quer {
				t.Fatalf("ipDe = %q, want %q", got, tc.quer)
			}
		})
	}
}

// A response above the ceiling can't arrive cut with a 200, nor enter the cache.
func TestRespostaGrandeVira502(t *testing.T) {
	for _, cache := range []string{"desligado", "ligado"} {
		t.Run("cache "+cache, func(t *testing.T) {
			c := novoCenario(t)
			_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
			resp := c.chat(chave, msg("enorme", "oi", false), "X-Comporta-Cache", cache)
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadGateway {
				t.Fatalf("status %d, want 502", resp.StatusCode)
			}
			if c.s.d.Cache.Bytes() != 0 {
				t.Fatal("cut response entered the cache")
			}
			// The provider charged for the whole response: the budget needs to see a cost, even an estimated one.
			if r := c.relatorio(); r.Total.CustoMicro == 0 {
				t.Fatal("large response recorded with zero cost")
			}
		})
	}
}

// DeepSeek's reasoning comes in delta.reasoning_content, with placeholders split across
// chunks like the content: it has to reach the client restored.
func TestStreamRestauraRaciocinio(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja", LGPD: true})
	resp := c.chat(chave, msg("pensa", "meu CPF é 529.982.247-25", true))
	defer resp.Body.Close()
	var raciocinio strings.Builder
	viuContent := false
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		dado, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok || dado == "[DONE]" {
			continue
		}
		var v struct {
			Choices []struct {
				Delta struct {
					Reasoning string `json:"reasoning_content"`
					Content   string
				}
			}
		}
		if json.Unmarshal([]byte(dado), &v) == nil && len(v.Choices) > 0 {
			d := v.Choices[0].Delta
			// The reasoning comes before the answer: a held-back piece can't come out after the content.
			if d.Reasoning != "" && viuContent {
				t.Fatalf("reasoning %q arrived after the content", d.Reasoning)
			}
			viuContent = viuContent || d.Content != ""
			raciocinio.WriteString(d.Reasoning)
		}
	}
	if got := raciocinio.String(); got != "o CPF é 529.982.247-25 mesmo [" {
		t.Fatalf("reasoning = %q", got)
	}
}

// On SIGTERM Cloud Run waits only a few seconds: a shadow stuck on a provider that doesn't
// answer can't hold shutdown until the total deadline (10 min).
func TestEncerrarCortaShadowPreso(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	conteudo(t, c.chat(chave, msg("presa", "oi", false)))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	fim := make(chan struct{})
	inicio := time.Now()
	go func() { c.s.Encerrar(ctx); close(fim) }()
	select {
	case <-fim:
	case <-time.After(3 * time.Second):
		t.Fatal("Encerrar did not respect the deadline: the stuck shadow held the shutdown")
	}
	if d := time.Since(inicio); d > 2*time.Second {
		t.Fatalf("Encerrar took %v", d)
	}
}

// On shutdown, a cut request (h.Close cancels its context) still needs to record usage before
// Encerrar returns: after that the writer is drained and the process exits.
func TestEncerrarEsperaRequisicaoCortada(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja"})
	req, _ := http.NewRequest("POST", c.url+"/v1/chat/completions", strings.NewReader(msg("calado", "oi", true)))
	req.Header.Set("Authorization", "Bearer "+chave)
	go func() {
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
	time.Sleep(300 * time.Millisecond) // the request gets in and is stuck on the mute provider
	c.http.CloseClientConnections()    // what h.Close() does to active connections
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // deadline already exceeded, as after a Shutdown that didn't finish
	c.s.Encerrar(ctx)
	if n := c.s.met.requisicoes.Load(); n != 1 {
		t.Fatalf("recorded requests = %d, want 1: Encerrar returned before the handler recorded usage", n)
	}
}

// The Playground shows what went out to the provider: the header carries the user's last message
// already masked (base64), and it only exists for clients with LGPD on.
func TestCabecalhoEnviadoMostraMascarado(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint("stream=", stream), func(t *testing.T) {
			c := novoCenario(t)
			_, comLGPD := c.novoCliente(armazenamento.Cliente{Nome: "loja", LGPD: true})
			_, semLGPD := c.novoCliente(armazenamento.Cliente{Nome: "outra"})

			resp := c.chat(comLGPD, msg("rapido", "meu CPF é 529.982.247-25", stream))
			resp.Body.Close()
			b, err := base64.StdEncoding.DecodeString(resp.Header.Get("X-Comporta-Enviado"))
			if err != nil || !strings.Contains(string(b), "[CPF_1]") || strings.Contains(string(b), "529.982.247-25") {
				t.Fatalf("header = %q (err %v): should carry the masked text", b, err)
			}

			resp = c.chat(semLGPD, msg("rapido", "oi", stream))
			resp.Body.Close()
			if h := resp.Header.Get("X-Comporta-Enviado"); h != "" {
				t.Fatalf("without LGPD there is nothing to show, got %q", h)
			}
		})
	}
}

// SDKs also send content as an array of parts. Text parts are masked like a string; a non-text
// part (image, audio) with LGPD on is refused instead of leaving unmasked. A null content
// (assistant tool call) stays null.
func TestLGPDConteudoEmPartes(t *testing.T) {
	c := novoCenario(t)
	_, chave := c.novoCliente(armazenamento.Cliente{Nome: "loja", LGPD: true})
	partes := `{"model":"rapido","messages":[
		{"role":"assistant","content":null},
		{"role":"user","content":[{"type":"text","text":"meu CPF é 529.982.247-25"},{"type":"text","text":"e-mail a@b.com"}]}]}`
	resp := c.chat(chave, partes)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("text parts: status %d, want 200", resp.StatusCode)
	}
	enviado := c.espiao.corpo()
	if strings.Contains(enviado, "529.982.247-25") || strings.Contains(enviado, "a@b.com") ||
		!strings.Contains(enviado, "[CPF_1]") || !strings.Contains(enviado, "[EMAIL_1]") {
		t.Fatalf("provider received personal data: %s", enviado)
	}
	if !strings.Contains(enviado, `"content":null`) {
		t.Fatalf("null content was rewritten: %s", enviado)
	}

	imagem := `{"model":"rapido","messages":[{"role":"user","content":[{"type":"text","text":"veja"},
		{"type":"image_url","image_url":{"url":"https://exemplo.com/rg.png"}}]}]}`
	resp = c.chat(chave, imagem)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("image part with LGPD on: status %d, want 400", resp.StatusCode)
	}
	if r := c.relatorio(); r.Total.PII != 2 { // relatorio shuts the server down: last
		t.Fatalf("PII = %d, want 2", r.Total.PII)
	}
}

// The Bearer header is a login too: without a limit, any /admin/* route would be an unlimited
// way to guess ADMIN_TOKEN. Wrong tokens spend the same per-IP limit as /admin/login; a correct
// token gives its token back, so automation with the right token isn't throttled.
func TestBearerErradoGastaOLimiteDeLogin(t *testing.T) {
	c := novoCenario(t)
	tentar := func(token, ip string) int {
		req, _ := http.NewRequest("GET", c.url+"/admin/clientes", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Forwarded-For", ip)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for range 10 {
		if st := tentar(tokenAdmin, "203.0.113.9"); st != 200 {
			t.Fatalf("correct Bearer: status %d, want 200 (must not spend the limit)", st)
		}
	}
	var ultimo int
	for range 6 {
		ultimo = tentar("chute", "203.0.113.9")
	}
	if ultimo != 429 {
		t.Fatalf("6th wrong Bearer from the same IP: status %d, want 429", ultimo)
	}
	if st := tentar(tokenAdmin, "203.0.113.9"); st != 429 {
		t.Fatalf("a right guess after the limit must not get in: status %d, want 429", st)
	}
	if st := tentar(tokenAdmin, "198.51.100.7"); st != 200 {
		t.Fatalf("admin on another IP was locked out: status %d", st)
	}
}

// 20 wrong Bearers at the same time from the same IP: at most 5 may reach the comparison (401);
// without check-compare-spend as one step, all of them would pass the check first.
func TestBearerParaleloRespeitaLimite(t *testing.T) {
	c := novoCenario(t)
	var wg sync.WaitGroup
	var comparadas atomic.Int32
	for range 20 {
		wg.Go(func() {
			req, _ := http.NewRequest("GET", c.url+"/admin/clientes", nil)
			req.Header.Set("Authorization", "Bearer chute")
			req.Header.Set("X-Forwarded-For", "203.0.113.9")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized {
				comparadas.Add(1)
			}
		})
	}
	wg.Wait()
	if n := comparadas.Load(); n != 5 {
		t.Fatalf("%d wrong Bearers reached the comparison; the limit is 5", n)
	}
}
