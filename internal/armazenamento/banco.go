// Package armazenamento guarda clientes, chaves e uso no SQLite (modernc, sem cgo).
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
	"time"

	_ "modernc.org/sqlite"
)

var ErrNaoEncontrado = errors.New("não encontrado")

type Banco struct{ db *sql.DB }

// AUTOINCREMENT: sem ele o SQLite reaproveita o maior ID removido, e um cliente novo herdaria
// o histórico de uso, o gasto do mês e o limitador do cliente apagado.
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

// Abrir cria o banco se preciso. WAL deixa o painel ler enquanto o gravador escreve;
// busy_timeout evita "database is locked" quando as duas coisas coincidem.
func Abrir(caminho string) (*Banco, error) {
	db, err := sql.Open("sqlite", "file:"+caminho+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("abrindo banco: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("criando schema: %w", err)
	}
	return &Banco{db: db}, nil
}

func (b *Banco) Fechar() error { return b.db.Close() }

func (b *Banco) Ping(ctx context.Context) error { return b.db.PingContext(ctx) }

type Cliente struct {
	ID             int64    `json:"id"`
	Nome           string   `json:"nome"`
	RPM            int      `json:"rpm"`             // 0 = sem limite
	OrcamentoMicro int64    `json:"orcamento_micro"` // por mês; 0 = sem limite
	Cache          bool     `json:"cache"`           // cache ligado por padrão para este cliente
	LGPD           bool     `json:"lgpd"`
	Modelos        []string `json:"modelos"` // aliases liberados; vazio = todos
}

func (b *Banco) CriarCliente(ctx context.Context, c Cliente) (Cliente, error) {
	m, _ := json.Marshal(c.Modelos)
	res, err := b.db.ExecContext(ctx,
		`INSERT INTO clientes (nome, rpm, orcamento_micro, cache, lgpd, modelos, criado_em) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.Nome, c.RPM, c.OrcamentoMicro, c.Cache, c.LGPD, string(m), time.Now().Unix())
	if err != nil {
		return c, fmt.Errorf("criando cliente: %w", err)
	}
	c.ID, err = res.LastInsertId()
	return c, err
}

func (b *Banco) AtualizarCliente(ctx context.Context, c Cliente) error {
	m, _ := json.Marshal(c.Modelos)
	res, err := b.db.ExecContext(ctx,
		`UPDATE clientes SET nome = ?, rpm = ?, orcamento_micro = ?, cache = ?, lgpd = ?, modelos = ? WHERE id = ?`,
		c.Nome, c.RPM, c.OrcamentoMicro, c.Cache, c.LGPD, string(m), c.ID)
	return linhaAfetada(res, err, "atualizando cliente")
}

func (b *Banco) RemoverCliente(ctx context.Context, id int64) error {
	res, err := b.db.ExecContext(ctx, `DELETE FROM clientes WHERE id = ?`, id)
	return linhaAfetada(res, err, "removendo cliente")
}

func (b *Banco) ListarClientes(ctx context.Context) ([]Cliente, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT id, nome, rpm, orcamento_micro, cache, lgpd, modelos FROM clientes ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("listando clientes: %w", err)
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

// HashChave é o que fica no banco. SHA-256 sem sal basta aqui: a chave tem 128 bits
// aleatórios, então não existe dicionário para atacar (diferente de uma senha).
func HashChave(texto string) string {
	h := sha256.Sum256([]byte(texto))
	return hex.EncodeToString(h[:])
}

// CriarChave devolve o texto inteiro da chave uma única vez; depois disso só existe o hash.
func (b *Banco) CriarChave(ctx context.Context, clienteID int64) (string, Chave, error) {
	texto := "ck_" + rand.Text()
	agora := time.Now()
	k := Chave{ClienteID: clienteID, Prefixo: texto[:7] + "…", CriadaEm: agora}
	res, err := b.db.ExecContext(ctx, `INSERT INTO chaves (cliente_id, prefixo, hash, criada_em) VALUES (?, ?, ?, ?)`,
		clienteID, k.Prefixo, HashChave(texto), agora.Unix())
	if err != nil {
		return "", k, fmt.Errorf("criando chave: %w", err)
	}
	k.ID, err = res.LastInsertId()
	return texto, k, err
}

// RevogarChave só revoga a chave se ela for do cliente: o id do cliente vem do path do admin.
func (b *Banco) RevogarChave(ctx context.Context, clienteID, id int64) error {
	res, err := b.db.ExecContext(ctx, `UPDATE chaves SET revogada_em = ? WHERE id = ? AND cliente_id = ? AND revogada_em IS NULL`,
		time.Now().Unix(), id, clienteID)
	return linhaAfetada(res, err, "revogando chave")
}

func (b *Banco) ListarChaves(ctx context.Context, clienteID int64) ([]Chave, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT id, cliente_id, prefixo, criada_em, revogada_em FROM chaves WHERE cliente_id = ? ORDER BY id`, clienteID)
	if err != nil {
		return nil, fmt.Errorf("listando chaves: %w", err)
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

// ChavesAtivas devolve hash → cliente. O servidor guarda isso num snapshot em memória para
// autenticar sem ir ao banco em toda requisição.
func (b *Banco) ChavesAtivas(ctx context.Context) (map[string]int64, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT hash, cliente_id FROM chaves WHERE revogada_em IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("lendo chaves ativas: %w", err)
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
