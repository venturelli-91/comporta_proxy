// Package armazenamento (storage) keeps clients, keys and usage in SQLite (modernc, no cgo).
package armazenamento

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNaoEncontrado = errors.New("not found")

type Banco struct{ db *sql.DB }

// AUTOINCREMENT: without it SQLite reuses the highest deleted ID, and a new client would inherit
// the deleted client's usage history, month's spend and limiter.
const schema = `
CREATE TABLE IF NOT EXISTS clientes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	nome TEXT NOT NULL,
	rpm INTEGER NOT NULL DEFAULT 0,
	orcamento_micro INTEGER NOT NULL DEFAULT 0,
	cache INTEGER NOT NULL DEFAULT 0,
	lgpd INTEGER NOT NULL DEFAULT 0,
	modelos TEXT NOT NULL DEFAULT '[]',
	criado_em INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS chaves (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	cliente_id INTEGER NOT NULL REFERENCES clientes(id) ON DELETE CASCADE,
	prefixo TEXT NOT NULL,
	hash TEXT NOT NULL UNIQUE,
	criada_em INTEGER NOT NULL,
	revogada_em INTEGER
);
CREATE TABLE IF NOT EXISTS uso (
	ts INTEGER NOT NULL,
	cliente_id INTEGER NOT NULL,
	request_id TEXT NOT NULL DEFAULT '',
	modelo TEXT NOT NULL DEFAULT '',
	destino TEXT NOT NULL DEFAULT '',
	status INTEGER NOT NULL,
	prompt_tokens INTEGER NOT NULL DEFAULT 0,
	completion_tokens INTEGER NOT NULL DEFAULT 0,
	cache_hit_tokens INTEGER NOT NULL DEFAULT 0,
	reasoning_tokens INTEGER NOT NULL DEFAULT 0,
	custo_micro INTEGER NOT NULL DEFAULT 0,
	economia_micro INTEGER NOT NULL DEFAULT 0,
	latencia_ms INTEGER NOT NULL DEFAULT 0,
	ttft_ms INTEGER NOT NULL DEFAULT 0,
	cache INTEGER NOT NULL DEFAULT 0,
	stream INTEGER NOT NULL DEFAULT 0,
	shadow INTEGER NOT NULL DEFAULT 0,
	pii INTEGER NOT NULL DEFAULT 0,
	tamanho_resposta INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS uso_ts ON uso(ts);
`

// Abrir (open) creates the database if needed. WAL lets the dashboard read while the writer writes;
// busy_timeout avoids "database is locked" when both happen at once.
func Abrir(caminho string) (*Banco, error) {
	db, err := sql.Open("sqlite", "file:"+caminho+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}
	return &Banco{db: db}, nil
}

func (b *Banco) Fechar() error { return b.db.Close() }

func (b *Banco) Ping(ctx context.Context) error { return b.db.PingContext(ctx) }

type Cliente struct {
	ID             int64    `json:"id"`
	Nome           string   `json:"nome"`
	RPM            int      `json:"rpm"`             // 0 = unlimited
	OrcamentoMicro int64    `json:"orcamento_micro"` // per month; 0 = unlimited
	Cache          bool     `json:"cache"`           // cache on by default for this client
	LGPD           bool     `json:"lgpd"`
	Modelos        []string `json:"modelos"` // allowed aliases; empty = all
}

func (b *Banco) CriarCliente(ctx context.Context, c Cliente) (Cliente, error) {
	m, _ := json.Marshal(c.Modelos)
	res, err := b.db.ExecContext(ctx,
		`INSERT INTO clientes (nome, rpm, orcamento_micro, cache, lgpd, modelos, criado_em) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.Nome, c.RPM, c.OrcamentoMicro, c.Cache, c.LGPD, string(m), time.Now().Unix())
	if err != nil {
		return c, fmt.Errorf("creating client: %w", err)
	}
	c.ID, err = res.LastInsertId()
	return c, err
}

func (b *Banco) AtualizarCliente(ctx context.Context, c Cliente) error {
	m, _ := json.Marshal(c.Modelos)
	res, err := b.db.ExecContext(ctx,
		`UPDATE clientes SET nome = ?, rpm = ?, orcamento_micro = ?, cache = ?, lgpd = ?, modelos = ? WHERE id = ?`,
		c.Nome, c.RPM, c.OrcamentoMicro, c.Cache, c.LGPD, string(m), c.ID)
	return linhaAfetada(res, err, "updating client")
}

func (b *Banco) RemoverCliente(ctx context.Context, id int64) error {
	res, err := b.db.ExecContext(ctx, `DELETE FROM clientes WHERE id = ?`, id)
	return linhaAfetada(res, err, "removing client")
}

func (b *Banco) ListarClientes(ctx context.Context) ([]Cliente, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT id, nome, rpm, orcamento_micro, cache, lgpd, modelos FROM clientes ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("listing clients: %w", err)
	}
	defer rows.Close()
	out := []Cliente{}
	for rows.Next() {
		var c Cliente
		var m string
		if err := rows.Scan(&c.ID, &c.Nome, &c.RPM, &c.OrcamentoMicro, &c.Cache, &c.LGPD, &m); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(m), &c.Modelos)
		out = append(out, c)
	}
	return out, rows.Err()
}

type Chave struct {
	ID         int64      `json:"id"`
	ClienteID  int64      `json:"cliente_id"`
	Prefixo    string     `json:"prefixo"`
	CriadaEm   time.Time  `json:"criada_em"`
	RevogadaEm *time.Time `json:"revogada_em"`
}

// HashChave is what goes into the database. Unsalted SHA-256 is enough here: the key has 128 random
// bits, so there's no dictionary to attack (unlike a password).
func HashChave(texto string) string {
	h := sha256.Sum256([]byte(texto))
	return hex.EncodeToString(h[:])
}

// CriarChave (create key) returns the full key text exactly once; after that only the hash exists.
func (b *Banco) CriarChave(ctx context.Context, clienteID int64) (string, Chave, error) {
	return b.registrarChave(ctx, clienteID, "ck_"+rand.Text())
}

// SemearDemo (seed demo) creates the "demo" client with the given key (the one published in the demo's
// README) when the database has no clients, which is the demo's state every time the ephemeral disk is
// wiped. It returns whether it created it. A public key is only acceptable because the demo uses the fake provider.
func (b *Banco) SemearDemo(ctx context.Context, chave string) (bool, error) {
	if !strings.HasPrefix(chave, "ck_") {
		return false, fmt.Errorf("the demo key must start with ck_")
	}
	var n int
	if err := b.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM clientes`).Scan(&n); err != nil || n > 0 {
		return false, err
	}
	c, err := b.CriarCliente(ctx, Cliente{Nome: "demo", RPM: 30, OrcamentoMicro: 1_000_000, LGPD: true})
	if err != nil {
		return false, err
	}
	_, _, err = b.registrarChave(ctx, c.ID, chave)
	return err == nil, err
}

func (b *Banco) registrarChave(ctx context.Context, clienteID int64, texto string) (string, Chave, error) {
	agora := time.Now()
	k := Chave{ClienteID: clienteID, Prefixo: texto[:7] + "…", CriadaEm: agora}
	res, err := b.db.ExecContext(ctx, `INSERT INTO chaves (cliente_id, prefixo, hash, criada_em) VALUES (?, ?, ?, ?)`,
		clienteID, k.Prefixo, HashChave(texto), agora.Unix())
	if err != nil {
		return "", k, fmt.Errorf("creating key: %w", err)
	}
	k.ID, err = res.LastInsertId()
	return texto, k, err
}

// RevogarChave (revoke key) only revokes the key if it belongs to the client: the client id comes from the admin path.
func (b *Banco) RevogarChave(ctx context.Context, clienteID, id int64) error {
	res, err := b.db.ExecContext(ctx, `UPDATE chaves SET revogada_em = ? WHERE id = ? AND cliente_id = ? AND revogada_em IS NULL`,
		time.Now().Unix(), id, clienteID)
	return linhaAfetada(res, err, "revoking key")
}

func (b *Banco) ListarChaves(ctx context.Context, clienteID int64) ([]Chave, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT id, cliente_id, prefixo, criada_em, revogada_em FROM chaves WHERE cliente_id = ? ORDER BY id`, clienteID)
	if err != nil {
		return nil, fmt.Errorf("listing keys: %w", err)
	}
	defer rows.Close()
	out := []Chave{}
	for rows.Next() {
		var k Chave
		var criada int64
		var revogada sql.NullInt64
		if err := rows.Scan(&k.ID, &k.ClienteID, &k.Prefixo, &criada, &revogada); err != nil {
			return nil, err
		}
		k.CriadaEm = time.Unix(criada, 0)
		if revogada.Valid {
			t := time.Unix(revogada.Int64, 0)
			k.RevogadaEm = &t
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ChavesAtivas (active keys) returns hash → client. The server keeps this in an in-memory snapshot
// to authenticate without hitting the database on every request.
func (b *Banco) ChavesAtivas(ctx context.Context) (map[string]int64, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT hash, cliente_id FROM chaves WHERE revogada_em IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("reading active keys: %w", err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var h string
		var id int64
		if err := rows.Scan(&h, &id); err != nil {
			return nil, err
		}
		out[h] = id
	}
	return out, rows.Err()
}

func linhaAfetada(res sql.Result, err error, op string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%s: %w", op, ErrNaoEncontrado)
	}
	return nil
}
