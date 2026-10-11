package armazenamento

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

// Evento (event) is a served call. It never stores message content, only measurements.
type Evento struct {
	TS                                                              time.Time
	ClienteID                                                       int64
	RequestID, Modelo, Destino                                      string
	Status                                                          int
	PromptTokens, CompletionTokens, CacheHitTokens, ReasoningTokens int64
	CustoMicro, EconomiaMicro                                       int64
	LatenciaMs, TTFTMs, TamanhoResposta                             int64
	Cache, Stream, Shadow                                           bool
	PII                                                             int
}

// Gravador (writer) takes database writes off the request path: the handler only puts the event on a
// channel, and a goroutine writes in batches (500 events or 1 s, whichever comes first).
type Gravador struct {
	b           *Banco
	ch          chan Evento
	descartados atomic.Int64
}

func NovoGravador(b *Banco, capacidade int) *Gravador {
	return &Gravador{b: b, ch: make(chan Evento, capacidade)}
}

// Registrar (record) never blocks. A full channel = the database can't keep up; losing a usage record
// is acceptable, blocking the API is not. Drops are counted and show up in /metrics.
func (g *Gravador) Registrar(e Evento) bool {
	select {
	case g.ch <- e:
		return true
	default:
		g.descartados.Add(1)
		return false
	}
}

func (g *Gravador) Descartados() int64 { return g.descartados.Load() }

// Rodar (run) writes until the context ends and then drains what's left in the channel: on a
// graceful shutdown no event already accepted is lost.
func (g *Gravador) Rodar(ctx context.Context) {
	const tamLote = 500
	lote := make([]Evento, 0, tamLote)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	gravar := func() {
		if len(lote) == 0 {
			return
		}
		// Its own context: the final batch is written even with Rodar's ctx already cancelled.
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := g.b.gravar(c, lote); err != nil {
			// A lost batch counts as dropped: it shows up in /metrics along with the full channel.
			g.descartados.Add(int64(len(lote)))
			slog.Error("writing usage", "events", len(lote), "error", err.Error())
		}
		lote = lote[:0]
	}
	for {
		select {
		case e := <-g.ch:
			if lote = append(lote, e); len(lote) == tamLote {
				gravar()
			}
		case <-tick.C:
			gravar()
		case <-ctx.Done():
			for {
				select {
				case e := <-g.ch:
					lote = append(lote, e)
				default:
					gravar()
					return
				}
			}
		}
	}
}

func (b *Banco) gravar(ctx context.Context, lote []Evento) error {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("writing usage: %w", err)
	}
	defer tx.Rollback()
	st, err := tx.PrepareContext(ctx, `INSERT INTO uso (ts, cliente_id, request_id, modelo, destino, status,
		prompt_tokens, completion_tokens, cache_hit_tokens, reasoning_tokens, custo_micro, economia_micro,
		latencia_ms, ttft_ms, cache, stream, shadow, pii, tamanho_resposta)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("writing usage: %w", err)
	}
	defer st.Close()
	for _, e := range lote {
		if _, err := st.ExecContext(ctx, e.TS.UnixMilli(), e.ClienteID, e.RequestID, e.Modelo, e.Destino, e.Status,
			e.PromptTokens, e.CompletionTokens, e.CacheHitTokens, e.ReasoningTokens, e.CustoMicro, e.EconomiaMicro,
			e.LatenciaMs, e.TTFTMs, e.Cache, e.Stream, e.Shadow, e.PII, e.TamanhoResposta); err != nil {
			return fmt.Errorf("writing usage: %w", err)
		}
	}
	return tx.Commit()
}

type Totais struct {
	Requisicoes   int64 `json:"requisicoes"`
	Erros         int64 `json:"erros"`
	CacheHits     int64 `json:"cache_hits"`
	Tokens        int64 `json:"tokens"`
	CustoMicro    int64 `json:"custo_micro"`
	EconomiaMicro int64 `json:"economia_micro"`
	PII           int64 `json:"pii"`
	P95Ms         int64 `json:"p95_ms"`
}

type Grupo struct {
	Nome string `json:"nome"`
	Totais
}

type Ponto struct {
	Minuto      int64 `json:"minuto"` // unix seconds
	Requisicoes int64 `json:"requisicoes"`
	Erros       int64 `json:"erros"`
}

type Comparacao struct {
	Modelo          string `json:"modelo"`
	Destino         string `json:"destino"`
	Shadow          bool   `json:"shadow"`
	Requisicoes     int64  `json:"requisicoes"`
	LatenciaMediaMs int64  `json:"latencia_media_ms"`
	CustoMedioMicro int64  `json:"custo_medio_micro"`
	TamanhoMedio    int64  `json:"tamanho_medio"`
}

type Relatorio struct {
	Total      Totais       `json:"total"`
	PorCliente []Grupo      `json:"por_cliente"`
	PorModelo  []Grupo      `json:"por_modelo"`
	Serie      []Ponto      `json:"serie"`
	Shadow     []Comparacao `json:"shadow"`
}

const somas = `count(*), coalesce(sum(uso.status >= 400), 0), coalesce(sum(uso.cache), 0),
	coalesce(sum(uso.prompt_tokens + uso.completion_tokens), 0), coalesce(sum(uso.custo_micro), 0),
	coalesce(sum(uso.economia_micro), 0), coalesce(sum(uso.pii), 0)`

// Relatorio (report) aggregates usage over [de, ate). cliente 0 = all. Shadow calls stay out of the
// totals (they weren't answered to anyone) and only appear in the comparison.
// ponytail: scans the raw table; the uso_minuto table comes in when this shows up in a profile.
func (b *Banco) Relatorio(ctx context.Context, de, ate time.Time, cliente int64) (Relatorio, error) {
	filtro := `ts >= ? AND ts < ? AND (? = 0 OR cliente_id = ?)`
	args := []any{de.UnixMilli(), ate.UnixMilli(), cliente, cliente}
	var r Relatorio

	t := &r.Total
	err := b.db.QueryRowContext(ctx, `SELECT `+somas+` FROM uso WHERE shadow = 0 AND `+filtro, args...).
		Scan(&t.Requisicoes, &t.Erros, &t.CacheHits, &t.Tokens, &t.CustoMicro, &t.EconomiaMicro, &t.PII)
	if err != nil {
		return r, fmt.Errorf("report: %w", err)
	}
	// Nearest-rank p95: the value at position ceil(0.95·n).
	if t.Requisicoes > 0 {
		pos := (95*t.Requisicoes+99)/100 - 1
		err = b.db.QueryRowContext(ctx, `SELECT latencia_ms FROM uso WHERE shadow = 0 AND `+filtro+
			` ORDER BY latencia_ms LIMIT 1 OFFSET ?`, append(args, pos)...).Scan(&t.P95Ms)
		if err != nil {
			return r, fmt.Errorf("report p95: %w", err)
		}
	}

	grupo := func(coluna string) ([]Grupo, error) {
		rows, err := b.db.QueryContext(ctx, `SELECT `+coluna+`, `+somas+` FROM uso LEFT JOIN clientes c ON c.id = uso.cliente_id
			WHERE shadow = 0 AND `+filtro+` GROUP BY 1 ORDER BY 2 DESC, 1`, args...)
		if err != nil {
			return nil, fmt.Errorf("report by group: %w", err)
		}
		defer rows.Close()
		out := []Grupo{}
		for rows.Next() {
			var g Grupo
			if err := rows.Scan(&g.Nome, &g.Requisicoes, &g.Erros, &g.CacheHits, &g.Tokens, &g.CustoMicro, &g.EconomiaMicro, &g.PII); err != nil {
				return nil, err
			}
			out = append(out, g)
		}
		return out, rows.Err()
	}
	if r.PorCliente, err = grupo(`coalesce(c.nome, 'removido')`); err != nil {
		return r, err
	}
	if r.PorModelo, err = grupo(`modelo`); err != nil {
		return r, err
	}

	rows, err := b.db.QueryContext(ctx, `SELECT ts / 60000 * 60, count(*), coalesce(sum(status >= 400), 0)
		FROM uso WHERE shadow = 0 AND `+filtro+` GROUP BY 1 ORDER BY 1`, args...)
	if err != nil {
		return r, fmt.Errorf("report series: %w", err)
	}
	defer rows.Close()
	r.Serie = []Ponto{}
	for rows.Next() {
		var p Ponto
		if err := rows.Scan(&p.Minuto, &p.Requisicoes, &p.Erros); err != nil {
			return r, err
		}
		r.Serie = append(r.Serie, p)
	}
	if err := rows.Err(); err != nil {
		return r, err
	}

	// Comparison only for the aliases that had shadows in the period: primary × shadow.
	rows2, err := b.db.QueryContext(ctx, `SELECT modelo, destino, shadow, count(*), avg(latencia_ms), avg(custo_micro), avg(tamanho_resposta)
		FROM uso WHERE status < 400 AND cache = 0 AND `+filtro+`
		AND modelo IN (SELECT DISTINCT modelo FROM uso WHERE shadow = 1 AND `+filtro+`)
		GROUP BY 1, 2, 3 ORDER BY 1, 3`, append(args, args...)...)
	if err != nil {
		return r, fmt.Errorf("report shadow: %w", err)
	}
	defer rows2.Close()
	r.Shadow = []Comparacao{}
	for rows2.Next() {
		var c Comparacao
		var lat, custo, tam float64
		if err := rows2.Scan(&c.Modelo, &c.Destino, &c.Shadow, &c.Requisicoes, &lat, &custo, &tam); err != nil {
			return r, err
		}
		c.LatenciaMediaMs, c.CustoMedioMicro, c.TamanhoMedio = int64(lat), int64(custo), int64(tam)
		r.Shadow = append(r.Shadow, c)
	}
	return r, rows2.Err()
}

// GastoDoMes (month's spend) sums the cost per client since the start of t's (UTC) month, for the budget.
func (b *Banco) GastoDoMes(ctx context.Context, t time.Time) (map[int64]int64, error) {
	t = t.UTC()
	inicio := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	rows, err := b.db.QueryContext(ctx, `SELECT cliente_id, sum(custo_micro) FROM uso WHERE shadow = 0 AND ts >= ? GROUP BY 1`, inicio.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("month spend: %w", err)
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var id, soma int64
		if err := rows.Scan(&id, &soma); err != nil {
			return nil, err
		}
		out[id] = soma
	}
	return out, rows.Err()
}
