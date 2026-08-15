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

// jobColumns lista as colunas lidas por todo SELECT que reconstrói um *Job
// (scanJob). Centralizado aqui porque -list-staged/-approve/-reject (Fase 1)
// e -history (Fase 6) rodam num processo CLI separado do que criou o job —
// o banco é a única fonte de verdade, então scanJob precisa trazer TODAS as
// colunas persistidas (started_at/finished_at/métricas/error), não só as
// usadas pelo caminho de execução original (NextPendingJob etc.).
const jobColumns = `id, path, status, driver, target, media_info, priority, created_at,
	started_at, finished_at, original_size, converted_size, saved_bytes, ratio_pct, error`

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

// UpdatePath persiste um novo caminho para o Job. Usado quando o container
// alvo muda a extensão do arquivo (Cleanup.FinalPath() difere do path
// original) — sem isso o registro no banco ficaria apontando para um
// arquivo que não existe mais em disco após o Commit.
func (s *Store) UpdatePath(id, path string) error {
	_, err := s.db.Exec(`UPDATE jobs SET path=? WHERE id=?`, path, id)
	return err
}

// UpdateMetrics persiste as métricas de eficiência de espaço do Job (seção 7).
func (s *Store) UpdateMetrics(id string, m SizeMetrics) error {
	_, err := s.db.Exec(
		`UPDATE jobs SET original_size=?, converted_size=?, saved_bytes=?, ratio_pct=? WHERE id=?`,
		m.OriginalSizeBytes, m.ConvertedSizeBytes, m.SavedBytes, m.CompressionRatioPct, id,
	)
	return err
}

// GetTotalSavings calcula a economia de espaço consolidada (somatória de todos os jobs COMPLETED).
func (s *Store) GetTotalSavings() (SizeMetrics, error) {
	row := s.db.QueryRow(
		`SELECT COALESCE(SUM(original_size), 0), COALESCE(SUM(converted_size), 0), COALESCE(SUM(saved_bytes), 0)
		 FROM jobs
		 WHERE status = ?`, StatusCompleted,
	)
	var m SizeMetrics
	err := row.Scan(&m.OriginalSizeBytes, &m.ConvertedSizeBytes, &m.SavedBytes)
	if err != nil {
		return m, err
	}
	if m.OriginalSizeBytes > 0 {
		m.CompressionRatioPct = (float64(m.SavedBytes) / float64(m.OriginalSizeBytes)) * 100.0
	}
	return m, nil
}

// NextPendingJob retorna o próximo Job aguardando processamento, por prioridade
// (maior prioridade primeiro) e em ordem de criação (FIFO como desempate).
func (s *Store) NextPendingJob() (*Job, error) {
	row := s.db.QueryRow(
		`SELECT `+jobColumns+`
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
		`SELECT `+jobColumns+`
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

// PendingJobCount retorna quantos jobs ainda aguardando processamento (QUEUED)
// ou em andamento (IN_PROGRESS). Usado pelo modo one-shot para aguardar a fila
// esvaziar antes de encerrar.
func (s *Store) PendingJobCount() (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM jobs WHERE status = ? OR status = ?`,
		StatusQueued, StatusInProgress,
	).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
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
		`SELECT `+jobColumns+`
		 FROM jobs WHERE path=? ORDER BY created_at DESC LIMIT 1`, path,
	)
	return scanJob(row)
}

// FindByID busca um Job pelo seu identificador único. Retorna (nil, nil)
// quando não encontrado (mesma convenção de scanJob/FindByPath).
func (s *Store) FindByID(id string) (*Job, error) {
	row := s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE id=?`, id)
	return scanJob(row)
}

// ListByStatus lista todos os Jobs num status específico, em ordem de
// criação (FIFO) — usado por ListStaged (AWAITING_APPROVAL) e, na Fase 6,
// como base de ListJobs.
func (s *Store) ListByStatus(status JobStatus) ([]*Job, error) {
	rows, err := s.db.Query(
		`SELECT `+jobColumns+` FROM jobs WHERE status=? ORDER BY created_at ASC`, status,
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

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanJob(row rowScanner) (*Job, error) {
	var (
		j                  Job
		targetRaw, miRaw   string
		createdRaw         string
		startedRaw, finRaw sql.NullString
		errMsg             sql.NullString
		origSize, convSize sql.NullInt64
		savedBytes         sql.NullInt64
		ratioPct           sql.NullFloat64
	)
	err := row.Scan(&j.ID, &j.Path, &j.Status, &j.Driver, &targetRaw, &miRaw, &j.Priority, &createdRaw,
		&startedRaw, &finRaw, &origSize, &convSize, &savedBytes, &ratioPct, &errMsg)
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
	if finRaw.Valid {
		t, _ := time.Parse(time.RFC3339, finRaw.String)
		j.FinishedAt = &t
	}
	if errMsg.Valid {
		j.Error = errMsg.String
	}
	j.SizeMetrics = SizeMetrics{
		OriginalSizeBytes:   origSize.Int64,
		ConvertedSizeBytes:  convSize.Int64,
		SavedBytes:          savedBytes.Int64,
		CompressionRatioPct: ratioPct.Float64,
	}
	return &j, nil
}
