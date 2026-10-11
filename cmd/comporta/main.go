// Command comporta reads the environment, wires the dependencies and serves the API with graceful shutdown.
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
	slog.SetDefault(log) // packages without their own logger (usage writer) also log JSON
	if err := rodar(log); err != nil {
		log.Error("exiting", "error", err.Error())
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
			// Without a real key, the provider becomes the fake on loopback: run, test and measure without spending.
			base, err = subirFalso()
			if err != nil {
				return err
			}
			log.Warn("no key for the provider: using the fake provider", "provider", nome, "env", p.ChaveEnv)
		}
		clientes[nome] = provedor.Novo(base, chave, max(p.MaxConexoes, 10), cfg.Timeouts.Conexao())
	}

	banco, err := armazenamento.Abrir(cmp.Or(os.Getenv("DB_PATH"), "comporta.db"))
	if err != nil {
		return err
	}
	defer banco.Fechar()
	gravador := armazenamento.NovoGravador(banco, 10_000)
	// Public demo (Render): the disk is wiped on every restart, so the demo client is recreated at startup.
	if chave := os.Getenv("DEMO_CHAVE"); chave != "" {
		criou, err := banco.SemearDemo(context.Background(), chave)
		if err != nil {
			return fmt.Errorf("seeding the demo: %w", err)
		}
		if criou {
			log.Info("demo client created")
		}
	}

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		// Local convenience: without a configured token, generate one and print it once to the terminal.
		adminToken = rand.Text()
		fmt.Fprintf(os.Stderr, "ADMIN_TOKEN not set; token for this run: %s\n", adminToken)
	}
	srv, err := httpapi.Novo(httpapi.Deps{
		Config: cfg, Roteador: gateway.NovoRoteador(cfg, clientes), Banco: banco, Gravador: gravador,
		Cache: cache.Novo(cfg.Cache.MaxMB<<20, time.Duration(cfg.Cache.TTLs)*time.Second), AdminToken: adminToken, Painel: web.Painel(), Log: log,
		// K_SERVICE is set by Cloud Run; TRUST_PROXY=1 for another trusted proxy in front.
		ConfiarProxy: os.Getenv("K_SERVICE") != "" || os.Getenv("TRUST_PROXY") == "1",
	})
	if err != nil {
		return err
	}
	ctxUso, pararUso := context.WithCancel(context.Background())
	usoFim := make(chan struct{})
	go func() { gravador.Rodar(ctxUso); close(usoFim) }()
	// Runs last (defers are LIFO): drains usage before closing the database.
	defer func() { pararUso(); <-usoFim }()

	// No WriteTimeout: it would cut long streams. Deadlines live in the relay's watchdog.
	h := &http.Server{Addr: ":" + cmp.Or(os.Getenv("PORT"), "8080"), Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()
	erroServidor := make(chan error, 1)
	go func() { erroServidor <- h.ListenAndServe() }()
	log.Info("comporta listening", "address", h.Addr)

	select {
	case err := <-erroServidor:
		return err
	case <-ctx.Done():
	}
	// Graceful shutdown: stop accepting, wait for requests and shadows, then drain usage.
	// 8 s because Cloud Run sends SIGKILL 10 s after SIGTERM; that leaves time to write usage.
	log.Info("shutting down")
	ctxFim, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := h.Shutdown(ctxFim); err != nil && !errors.Is(err, http.ErrServerClosed) {
		// Deadline exceeded with open streams: Close cancels their context, and they charge the
		// (estimated) usage and record it before exiting. Otherwise the process would end mid-stream.
		log.Warn("shutdown", "error", err.Error())
		h.Close()
	}
	srv.Encerrar(ctxFim)
	return nil
}

func subirFalso() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("starting the fake provider: %w", err)
	}
	f := &provedor.Falso{
		Latencia: time.Duration(envNum("FALSO_LATENCIA_MS", 60)) * time.Millisecond,
		PorToken: time.Duration(envNum("FALSO_POR_TOKEN_MS", 5)) * time.Millisecond,
		TaxaErro: envNum("FALSO_TAXA_ERRO", 0),
	}
	go http.Serve(ln, f)
	return "http://" + ln.Addr().String(), nil
}
