package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Engine orquestra todo o ciclo de vida dos Jobs (seção 2, RF04-RF06).
type Engine struct {
	store     *Store
	rules     *RulesEngine
	prober    MediaProber
	engines   map[string]TranscoderEngine
	workers   int
	events    chan JobEvent
	staging   string
	integrity *IntegrityCheck
	log       io.Writer
	wg        sync.WaitGroup
	watcher   *Watcher
	cancelled chan struct{}
	closeOnce sync.Once
}

// EngineDeps agrupa as dependências para criar um Engine.
type EngineDeps struct {
	Store   *Store
	Rules   *RulesEngine
	Prober  MediaProber
	Engines []TranscoderEngine
	Workers int
	Events  chan JobEvent
	Log     io.Writer
}

// NewEngine constrói o orquestrador a partir das dependências.
func NewEngine(d EngineDeps) (*Engine, error) {
	if d.Workers < 1 {
		d.Workers = 1
	}
	if d.Log == nil {
		d.Log = os.Stderr
	}
	em := make(map[string]TranscoderEngine)
	for _, e := range d.Engines {
		em[e.Name()] = e
	}
	return &Engine{
		store:     d.Store,
		rules:     d.Rules,
		prober:    d.Prober,
		engines:   em,
		workers:   d.Workers,
		events:    d.Events,
		staging:   d.Rules.Global().StagingDir,
		integrity: NewIntegrityCheck(d.Rules.Global().SpaceSaving.MinSavingPct),
		log:       d.Log,
		cancelled: make(chan struct{}),
	}, nil
}

// Events returns o canal de eventos nativos (RF06, RI02).
func (e *Engine) Events() <-chan JobEvent { return e.events }
func (e *Engine) logf(format string, a ...interface{}) {
	fmt.Fprintf(e.log, "[codecany] "+format+"\n", a...)
}
func (e *Engine) emit(ev JobEvent) {
	if e.events != nil {
		e.events <- ev
	}
}

// HandleDiscovered processa um arquivo detectado pelo Watcher (RF01→RF03).
// Retorna true se um job foi criado (enfileirado).
func (e *Engine) HandleDiscovered(path string) bool {
	if e.rules.ShouldIgnore(path, 0) {
		return false
	}
	mi, err := e.prober.Probe(path)
	if err != nil {
		e.logf("probe falhou %s: %v", path, err)
		return false
	}
	outcome, target, err := e.rules.Evaluate(mi)
	if err != nil {
		e.logf("avaliar regras %s: %v", path, err)
		return false
	}
	if outcome != OutcomeConvert {
		e.logf("skip %s", path)
		return false
	}
	driver := e.rules.Global().DefaultDriver
	if _, ok := e.engines[driver]; !ok {
		e.logf("driver não registrado: %s", driver)
		return false
	}
	if existing, err := e.store.FindByPath(path); err == nil && existing != nil {
		// já existe job para este caminho
		if existing.Status != StatusFailed && existing.Status != StatusRolledBack {
			return false
		}
	}
	job := &Job{
		ID:        uuid.NewString(),
		Path:      path,
		Status:    StatusQueued,
		Driver:    driver,
		Target:    target,
		MediaInfo: mi,
		CreatedAt: time.Now(),
	}
	if err := e.store.CreateJob(job); err != nil {
		e.logf("criar job %s: %v", path, err)
		return false
	}
	e.logf("job enfileirado %s (%s)", path, job.ID[:8])
	return true
}

// startWorkers dispara o pool de workers e recupera jobs de run anterior.
func (e *Engine) startWorkers(ctx context.Context) {
	for i := 0; i < e.workers; i++ {
		e.wg.Add(1)
		go e.workerLoop(ctx)
	}
	if err := e.bootRecover(); err != nil {
		e.logf("recuperação de boot: %v", err)
	}
}

// OpenWorkers inicia o pool de workers consumindo a fila.
// Bloqueia até o contexto ser cancelado ou o canal de jobs ser fechado.
func (e *Engine) OpenWorkers(ctx context.Context) {
	e.startWorkers(ctx)
	e.wg.Wait()
}

func (e *Engine) bootRecover() error {
	jobs, err := e.store.RecoverInterrupted()
	if err != nil {
		return err
	}
	e.logf("recuperados %d jobs de run anterior", len(jobs))
	return nil
}

// RunOnce processa um conjunto específico de arquivos (modo one-shot) e retorna
// quando a fila esvaziar, em vez de monitorar diretórios indefinidamente.
// Retorna erro se algum arquivo informado não pôde ser processado.
func (e *Engine) RunOnce(ctx context.Context, files []string) error {
	var skipped int
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			e.logf("arquivo inacessível em -file: %s (%v)", f, err)
			skipped++
			continue
		}
		if fi.IsDir() {
			e.logf("skip diretório em -file: %s", f)
			skipped++
			continue
		}
		if !e.HandleDiscovered(f) {
			skipped++
		}
	}
	e.startWorkers(ctx)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		n, err := e.store.PendingJobCount()
		if err != nil {
			e.CloseCancellation()
			e.wg.Wait()
			return err
		}
		if n == 0 {
			break
		}
		select {
		case <-ctx.Done():
			e.CloseCancellation()
			e.wg.Wait()
			return ctx.Err()
		case <-tick.C:
		}
	}
	e.CloseCancellation()
	e.wg.Wait()
	if skipped > 0 {
		return fmt.Errorf("%d arquivo(s) não puderam ser processados", skipped)
	}
	return nil
}

// workerLoop consome a fila de jobs até o contexto ser cancelado ou abortado.
func (e *Engine) workerLoop(ctx context.Context) {
	defer e.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.cancelled:
			return
		default:
		}
		// espera por jobs
		job, err := e.store.NextPendingJob()
		if err != nil {
			e.logf("next job: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if job == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
				continue
			}
		}
		e.runJob(ctx, job)
	}
}

func (e *Engine) runJob(ctx context.Context, job *Job) {
	now := time.Now()
	if err := e.store.UpdateStatus(job.ID, StatusInProgress, &now, nil, ""); err != nil {
		e.logf("marcar in_progress %s: %v", job.ID, err)
		return
	}
	e.emit(JobEvent{Kind: EventJobStart, JobID: job.ID, FilePath: job.Path, Driver: job.Driver})

	cl, err := NewCleanup(job.Path, e.staging)
	if err != nil {
		e.fail(job, fmt.Sprintf("staging: %v", err))
		return
	}

	tc, ok := e.engines[job.Driver]
	if !ok {
		e.fail(job, "driver não registrado")
		return
	}

	prog, err := tc.Transcode(job.Path, cl.Output(), job.MediaInfo, job.Target)
	if err != nil {
		cl.Abort()
		e.fail(job, fmt.Sprintf("transcode: %v", err))
		return
	}

	// consome progresso
loop:
	for {
		select {
		case <-ctx.Done():
			cl.Abort()
			e.fail(job, "shutdown durante transcode")
			return
		case <-e.cancelled:
			cl.Abort()
			e.fail(job, "cancelado")
			return
		case fr, ok := <-prog:
			if !ok {
				break loop
			}
			e.emit(JobEvent{Kind: EventJobProgress, JobID: job.ID, FilePath: job.Path, Driver: job.Driver, Progress: fr})
		}
	}

	// TESTING
	nowT := time.Now()
	e.store.UpdateStatus(job.ID, StatusTesting, nil, &nowT, "")
	keep, m, err := e.integrity.Check(job.Path, cl.Output())
	if err != nil {
		cl.Abort()
		e.fail(job, fmt.Sprintf("verificação de integridade: %v", err))
		return
	}

	if !keep {
		// rollback
		cl.Abort()
		fin := time.Now()
		e.store.UpdateStatus(job.ID, StatusRolledBack, nil, &fin, "")
		e.logf("rollback %s: economia %.2f%% < %.2f%%", job.Path, m.CompressionRatioPct, e.integrity.MinSavingPct)
		e.emit(JobEvent{Kind: EventJobComplete, JobID: job.ID, FilePath: job.Path, Driver: job.Driver, Success: false, SizeDiff: m.SavedBytes})
		return
	}

	// FINALIZING → COMMIT
	e.store.UpdateStatus(job.ID, StatusFinalizing, nil, nil, "")
	if err := cl.Commit(); err != nil {
		e.fail(job, fmt.Sprintf("finalização: %v", err))
		return
	}
	e.store.UpdateMetrics(job.ID, m)
	fin := time.Now()
	e.store.UpdateStatus(job.ID, StatusCompleted, nil, &fin, "")
	e.logf("completo %s: salvo %d bytes (%.2f%%)", job.Path, m.SavedBytes, m.CompressionRatioPct)
	e.emit(JobEvent{Kind: EventJobComplete, JobID: job.ID, FilePath: job.Path, Driver: job.Driver, Success: true, SizeDiff: m.SavedBytes})
}

func (e *Engine) fail(job *Job, msg string) {
	fin := time.Now()
	_ = e.store.UpdateStatus(job.ID, StatusFailed, nil, &fin, msg)
	e.logf("falha %s: %s", job.Path, msg)
	e.emit(JobEvent{Kind: EventJobError, JobID: job.ID, FilePath: job.Path, Driver: job.Driver, Error: msg})
}

// CloseCancellation sinaliza os workers para abortar o job em andamento.
func (e *Engine) CloseCancellation() {
	e.closeOnce.Do(func() { close(e.cancelled) })
}

// StartWatcher inicializa o watcher com debounce e o liga ao engine (RF01).
func (e *Engine) StartWatcher(stableFor time.Duration) error {
	w, err := NewWatcher(stableFor, func(path string) { e.HandleDiscovered(path) })
	if err != nil {
		return err
	}
	e.watcher = w
	return nil
}

// WatchDir passa a monitorar um diretório (recursivo).
func (e *Engine) WatchDir(dir string) error {
	if e.watcher == nil {
		if err := e.StartWatcher(5e9); err != nil {
			return err
		}
	}
	return e.watcher.AddDir(dir)
}

// StartWatch inicia o loop do watcher em background.
func (e *Engine) StartWatch() {
	if e.watcher != nil {
		e.watcher.Start()
	}
}

// Shutdown encerra o watcher e o store.
func (e *Engine) Shutdown() {
	if e.watcher != nil {
		e.watcher.Close()
	}
	if e.store != nil {
		e.store.Close()
	}
}

// Graceful encerra os workers aguardando conclusão da fila.
func (e *Engine) Graceful(ctx context.Context) {
	e.CloseCancellation()
	<-ctx.Done()
}
