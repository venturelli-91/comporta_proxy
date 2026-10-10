// Comando comporta: lê o ambiente, liga as dependências e sobe a API com shutdown gracioso.
package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/venturelli-91/comporta_proxy/internal/armazenamento"
	"github.com/venturelli-91/comporta_proxy/internal/cache"
	"github.com/venturelli-91/comporta_proxy/internal/gateway"
	"github.com/venturelli-91/comporta_proxy/internal/httpapi"
	"github.com/venturelli-91/comporta_proxy/internal/provedor"
	"github.com/venturelli-91/comporta_proxy/web"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := rodar(log); err != nil {
		log.Error("encerrando", "erro", err.Error())
		os.Exit(1)
	}
}

func envNum(nome string, padrao float64) float64 {
	v, err := strconv.ParseFloat(os.Getenv(nome), 64)
	if err != nil {
		return padrao
	}
	return v
}

func rodar(log *slog.Logger) error {
	cfg, err := gateway.Carregar(cmp.Or(os.Getenv("CONFIG_PATH"), "comporta.json"))
	if err != nil {
		return err
	}

	clientes := map[string]*provedor.Cliente{}
	for nome, p := range cfg.Provedores {
		chave, base := os.Getenv(p.ChaveEnv), p.BaseURL
		if nome == "deepseek" {
			base = cmp.Or(os.Getenv("DEEPSEEK_BASE_URL"), base)
		}
		if chave == "" {
			// Sem chave real, o provedor vira o falso em loopback: dá para rodar, testar e medir sem gastar.
			base, err = subirFalso()
			if err != nil {
				return err
			}
			log.Warn("sem chave para o provedor: usando o provedor falso", "provedor", nome, "env", p.ChaveEnv)
		}
		clientes[nome] = provedor.Novo(base, chave, max(p.MaxConexoes, 10), cfg.Timeouts.Conexao())
	}

	banco, err := armazenamento.Abrir(cmp.Or(os.Getenv("DB_PATH"), "comporta.db"))
	if err != nil {
		return err
	}
	defer banco.Fechar()
	gravador := armazenamento.NovoGravador(banco, 10_000)

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		// Conveniência local: sem token configurado, gera um e mostra uma vez no terminal.
		adminToken = rand.Text()
		fmt.Fprintf(os.Stderr, "ADMIN_TOKEN não definido; token desta execução: %s\n", adminToken)
	}
	mb, ttl := cfg.Cache.MaxMB, time.Duration(cfg.Cache.TTLs)*time.Second
	if mb <= 0 {
		mb = 64
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	srv, err := httpapi.Novo(httpapi.Deps{
		Config: cfg, Roteador: gateway.NovoRoteador(cfg, clientes), Banco: banco, Gravador: gravador,
		Cache: cache.Novo(mb<<20, ttl), AdminToken: adminToken, Painel: web.Painel(), Log: log,
		// K_SERVICE é definido pelo Cloud Run; TRUST_PROXY=1 para outro proxy confiável na frente.
		ConfiarProxy: os.Getenv("K_SERVICE") != "" || os.Getenv("TRUST_PROXY") == "1",
	})
	if err != nil {
		return err
	}
	ctxUso, pararUso := context.WithCancel(context.Background())
	usoFim := make(chan struct{})
	go func() { gravador.Rodar(ctxUso); close(usoFim) }()
	// Roda por último (defers são LIFO): drena o uso antes de fechar o banco.
	defer func() { pararUso(); <-usoFim }()

	// Sem WriteTimeout: ele cortaria streams longos. Os prazos ficam no watchdog do relay.
	h := &http.Server{Addr: ":" + cmp.Or(os.Getenv("PORT"), "8080"), Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()
	erroServidor := make(chan error, 1)
	go func() { erroServidor <- h.ListenAndServe() }()
	log.Info("comporta no ar", "endereco", h.Addr)

	select {
	case err := <-erroServidor:
		return err
	case <-ctx.Done():
	}
	// Shutdown gracioso: para de aceitar, espera as requisições e os shadows, depois drena o uso.
	// 8 s porque o Cloud Run manda SIGKILL 10 s depois do SIGTERM; sobra tempo para gravar o uso.
	log.Info("desligando")
	ctxFim, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := h.Shutdown(ctxFim); err != nil && !errors.Is(err, http.ErrServerClosed) {
		// Prazo estourado com streams abertos: Close cancela o contexto deles, que cobram o uso
		// (estimado) e registram antes de sair. Sem isso o processo terminaria no meio deles.
		log.Warn("shutdown", "erro", err.Error())
		h.Close()
	}
	srv.Encerrar(ctxFim)
	return nil
}

func subirFalso() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("subindo provedor falso: %w", err)
	}
	f := &provedor.Falso{
		Latencia: time.Duration(envNum("FALSO_LATENCIA_MS", 60)) * time.Millisecond,
		PorToken: time.Duration(envNum("FALSO_POR_TOKEN_MS", 5)) * time.Millisecond,
		TaxaErro: envNum("FALSO_TAXA_ERRO", 0),
	}
	go http.Serve(ln, f)
	return "http://" + ln.Addr().String(), nil
}
