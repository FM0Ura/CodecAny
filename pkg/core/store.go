package core

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store gerencia a persistência dos Jobs no SQLite em modo WAL (RNF03, seção 4.1).
// Utiliza o driver modernc.org/sqlite (sem CGO), mantendo um único arquivo .db.
type Store struct {
	db *sql.DB
}

// NewStore abre (ou cria) o banco SQLite em modo WAL no path informado.
func NewStore(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("criar dir do store: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("abrir sqlite: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("ativar WAL: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("setar busy_timeout: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("criar schema: %w", err)
	}
	return &Store{db: db}, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS jobs (
    id          TEXT PRIMARY KEY,
    path        TEXT NOT NULL,
    status      TEXT NOT NULL,
    driver      TEXT NOT NULL,
    target      TEXT NOT NULL,
    media_info  TEXT NOT NULL,
    priority    INTEGER DEFAULT 0,
    original_size INTEGER DEFAULT 0,
    converted_size INTEGER DEFAULT 0,
    saved_bytes INTEGER DEFAULT 0,
    ratio_pct   REAL DEFAULT 0,
    created_at  TEXT NOT NULL,
    started_at  TEXT,
    finished_at TEXT,
    error       TEXT
);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_path ON jobs(path);
`

// Close encerra a conexão com o banco.
func (s *Store) Close() error { return s.db.Close() }

// CreateJob persiste um novo Job em estado QUEUED.
func (s *Store) CreateJob(job *Job) error {
	target, err := json.Marshal(job.Target)
	if err != nil {
		return err
	}
	mi, err := json.Marshal(job.MediaInfo)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO jobs (id, path, status, driver, target, media_info, priority, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Path, job.Status, job.Driver, string(target), string(mi),
		job.Priority, job.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

// UpdateStatus atualiza o estado de um Job e seus timestamps quando pertinente.
func (s *Store) UpdateStatus(id string, status JobStatus, startedAt, finishedAt *time.Time, errMsg string) error {
	var st, fi interface{}
	if startedAt != nil {
		st = startedAt.UTC().Format(time.RFC3339)
	}
	if finishedAt != nil {
		fi = finishedAt.UTC().Format(time.RFC3339)
	}
	sqlErr := ""
	if errMsg != "" {
		sqlErr = errMsg
	}
	res, err := s.db.Exec(
		`UPDATE jobs SET status=?, started_at=COALESCE(?, started_at),
		 finished_at=COALESCE(?, finished_at), error=? WHERE id=?`,
		status, st, fi, sqlErr, id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("job não encontrado")
	}
	return nil
}

// UpdateMetrics persiste as métricas de eficiência de espaço do Job (seção 7).
func (s *Store) UpdateMetrics(id string, m SizeMetrics) error {
	_, err := s.db.Exec(
		`UPDATE jobs SET original_size=?, converted_size=?, saved_bytes=?, ratio_pct=? WHERE id=?`,
		m.OriginalSizeBytes, m.ConvertedSizeBytes, m.SavedBytes, m.CompressionRatioPct, id,
	)
	return err
}

// NextPendingJob retorna o próximo Job aguardando processamento, por prioridade
// (maior prioridade primeiro) e em ordem de criação (FIFO como desempate).
func (s *Store) NextPendingJob() (*Job, error) {
	row := s.db.QueryRow(
		`SELECT id, path, status, driver, target, media_info, priority, created_at
		 FROM jobs
		 WHERE status = ?
		 ORDER BY priority DESC, created_at ASC
		 LIMIT 1`, StatusQueued,
	)
	return scanJob(row)
}

// ClaimPendingMarks marca todos os Jobs QUEUED como IN_PROGRESS para orquestração.
func (s *Store) ClaimPendingMarks() ([]*Job, error) {
	rows, err := s.db.Query(
		`SELECT id, path, status, driver, target, media_info, priority, created_at
		 FROM jobs WHERE status = ? ORDER BY priority DESC, created_at ASC`, StatusQueued,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// RecoverInterrupted devolve jobs pendentes de run anteriores (boot orphan recovery,
// seção 5.3) marcando-os como QUEUED para re-enfileiramento.
func (s *Store) RecoverInterrupted() ([]*Job, error) {
	_, err := s.db.Exec(
		`UPDATE jobs SET status=? WHERE status=? OR status=?`,
		StatusQueued, StatusInProgress, StatusQueued,
	)
	if err != nil {
		return nil, err
	}
	return s.ClaimPendingMarks()
}

// FindByPath returns o Job mais recente para um caminho, se existir.
func (s *Store) FindByPath(path string) (*Job, error) {
	row := s.db.QueryRow(
		`SELECT id, path, status, driver, target, media_info, priority, created_at
		 FROM jobs WHERE path=? ORDER BY created_at DESC LIMIT 1`, path,
	)
	return scanJob(row)
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanJob(row rowScanner) (*Job, error) {
	var (
		j                Job
		targetRaw, miRaw string
		createdRaw       string
		startedRaw       sql.NullString
	)
	err := row.Scan(&j.ID, &j.Path, &j.Status, &j.Driver, &targetRaw, &miRaw, &j.Priority, &createdRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	j.CreatedAt, _ = time.Parse(time.RFC3339, createdRaw)
	_ = json.Unmarshal([]byte(targetRaw), &j.Target)
	_ = json.Unmarshal([]byte(miRaw), &j.MediaInfo)
	if startedRaw.Valid {
		t, _ := time.Parse(time.RFC3339, startedRaw.String)
		j.StartedAt = &t
	}
	return &j, nil
}
