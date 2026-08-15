package core

import (
	"context"
	"fmt"
	"log/slog"
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
	verifier  MediaVerifier
	engines   map[string]TranscoderEngine
	workers   int
	events    chan JobEvent
	staging   string
	integrity *IntegrityCheck
	log       *slog.Logger
	scanEvery time.Duration
	wg        sync.WaitGroup
	watcher   *Watcher
	cancelled chan struct{}
	closeOnce sync.Once
	webhook   *WebhookClient
}

// EngineDeps agrupa as dependências para criar um Engine.
type EngineDeps struct {
	Store    *Store
	Rules    *RulesEngine
	Prober   MediaProber
	Verifier MediaVerifier
	Engines  []TranscoderEngine
	Workers  int
	Events   chan JobEvent
	Logger   *slog.Logger
}

// NewEngine constrói o orquestrador a partir das dependências.
func NewEngine(d EngineDeps) (*Engine, error) {
	if d.Workers < 1 {
		d.Workers = 1
	}
	if d.Logger == nil {
		d.Logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	em := make(map[string]TranscoderEngine)
	for _, e := range d.Engines {
		em[e.Name()] = e
	}
	return &Engine{
		store:     d.Store,
		rules:     d.Rules,
		prober:    d.Prober,
		verifier:  d.Verifier,
		engines:   em,
		workers:   d.Workers,
		events:    d.Events,
		staging:   d.Rules.Global().StagingDir,
		integrity: NewIntegrityCheck(d.Rules.Global().SpaceSaving.MinSavingPct),
		log:       d.Logger,
		scanEvery: defaultScanInterval,
		cancelled: make(chan struct{}),
		webhook:   NewWebhookClient(d.Rules.Global().Notifications),
	}, nil
}

// Events returns o canal de eventos nativos (RF06, RI02).
func (e *Engine) Events() <-chan JobEvent { return e.events }
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
		e.log.Warn("probe falhou", "path", path, "error", err.Error())
		return false
	}
	outcome, target, err := e.rules.Evaluate(mi)
	if err != nil {
		e.log.Warn("avaliar regras falhou", "path", path, "error", err.Error())
		return false
	}
	if outcome != OutcomeConvert {
		if outcome == OutcomeSkip {
			e.log.Info("regra solicitou skip", "path", path,
				"container", mi.Container, "video_codec", mi.VideoCodec,
				"audio_codecs", mi.AudioCodecs,
				"reason", e.rules.DescribeMiss(mi))
			return false
		}
		e.log.Info("nenhuma regra atendida - arquivo ignorado", "path", path,
			"container", mi.Container, "video_codec", mi.VideoCodec,
			"audio_codecs", mi.AudioCodecs,
			"reason", e.rules.DescribeMiss(mi))
		return false
	}
	driver := e.rules.Global().DefaultDriver
	if _, ok := e.engines[driver]; !ok {
		e.log.Error("driver não registrado", "path", path, "driver", driver)
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
		e.log.Error("falha ao criar job", "path", path, "error", err.Error())
		return false
	}
	e.log.Info("job enfileirado", "job_id", job.ID, "path", job.Path, "driver", job.Driver)
	return true
}

// startWorkers dispara o pool de workers e recupera jobs de run anterior.
func (e *Engine) startWorkers(ctx context.Context) {
	for i := 0; i < e.workers; i++ {
		e.wg.Add(1)
		go e.workerLoop(ctx)
	}
	if err := e.bootRecover(); err != nil {
		e.log.Error("recuperação de boot falhou", "error", err.Error())
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
	e.log.Info("recuperados jobs de run anterior", "count", len(jobs))
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
			e.log.Warn("arquivo inacessível em -file", "path", f, "error", err.Error())
			skipped++
			continue
		}
		if fi.IsDir() {
			e.log.Warn("skip diretório em -file", "path", f)
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
			e.log.Warn("falha ao buscar próximo job", "error", err.Error())
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
		e.log.Error("falha ao marcar in_progress", "job_id", job.ID, "error", err.Error())
		return
	}
	e.emit(JobEvent{Kind: EventJobStart, JobID: job.ID, FilePath: job.Path, Driver: job.Driver})
	e.log.Info("job iniciado", "job_id", job.ID, "path", job.Path, "driver", job.Driver)

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
			e.log.Debug("progresso transcode", "job_id", job.ID, "path", job.Path,
				"progress", fr)
			e.emit(JobEvent{Kind: EventJobProgress, JobID: job.ID, FilePath: job.Path, Driver: job.Driver, Progress: fr})
		}
	}

	// TESTING
	nowT := time.Now()
	e.store.UpdateStatus(job.ID, StatusTesting, nil, &nowT, "")

	// Verificação de integridade/segurança: o output convertido é decodificado
	// de ponta a ponta antes de qualquer troca. Se falhar, o original é
	// preservado (não substituímos um MKV bom por uma conversão quebrada).
	if e.verifier != nil {
		if err := e.verifier.Verify(cl.Output()); err != nil {
			cl.Abort()
			e.fail(job, fmt.Sprintf("verificação de decodificação: %v", err))
			return
		}
	}

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
		job.Status = StatusRolledBack
		job.SizeMetrics = m
		e.sendWebhook(job, "failed")
		e.log.Info("job revertido (economia insuficiente)",
			"job_id", job.ID, "path", job.Path,
			"savings_pct", m.CompressionRatioPct, "min_savings_pct", e.integrity.MinSavingPct,
			"saved_bytes", m.SavedBytes)
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
	job.Status = StatusCompleted
	job.SizeMetrics = m
	e.sendWebhook(job, "completed")
	e.log.Info("job completo", "job_id", job.ID, "path", job.Path, "driver", job.Driver,
		"saved_bytes", m.SavedBytes, "savings_pct", m.CompressionRatioPct,
		"duration_ms", time.Since(now).Milliseconds())
	e.emit(JobEvent{Kind: EventJobComplete, JobID: job.ID, FilePath: job.Path, Driver: job.Driver, Success: true, SizeDiff: m.SavedBytes})
}

func (e *Engine) fail(job *Job, msg string) {
	fin := time.Now()
	_ = e.store.UpdateStatus(job.ID, StatusFailed, nil, &fin, msg)
	job.Status = StatusFailed
	job.Error = msg
	e.sendWebhook(job, "failed")
	e.log.Error("job falhou", "job_id", job.ID, "path", job.Path, "driver", job.Driver, "error", msg)
	e.emit(JobEvent{Kind: EventJobError, JobID: job.ID, FilePath: job.Path, Driver: job.Driver, Error: msg})
}

// CloseCancellation sinaliza os workers para abortar o job em andamento.
func (e *Engine) CloseCancellation() {
	e.closeOnce.Do(func() { close(e.cancelled) })
}

// StartWatcher inicializa o watcher com debounce e o liga ao engine (RF01).
func (e *Engine) StartWatcher(stableFor time.Duration) error {
	return e.StartWatcherWithScan(stableFor, defaultScanInterval)
}

// defaultScanInterval é o intervalo padrão da varredura periódica de fallback.
const defaultScanInterval = time.Minute

// StartWatcherWithScan inicializa o watcher com debounce e varredura periódica.
func (e *Engine) StartWatcherWithScan(stableFor, scanEvery time.Duration) error {
	w, err := NewWatcher(stableFor, func(path string) { e.HandleDiscovered(path) }, e.log, scanEvery)
	if err != nil {
		return err
	}
	e.watcher = w
	return nil
}

// SetScanInterval define o intervalo da varredura periódica de fallback.
// Deve ser chamado antes de WatchDir; 0 desliga a varredura.
func (e *Engine) SetScanInterval(d time.Duration) { e.scanEvery = d }

// WatchDir passa a monitorar um diretório (recursivo).
func (e *Engine) WatchDir(dir string) error {
	if e.watcher == nil {
		if err := e.StartWatcherWithScan(5e9, e.scanEvery); err != nil {
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

// GetTotalSavings delega ao store a consulta consolidada de economias.
func (e *Engine) GetTotalSavings() (SizeMetrics, error) {
	if e.store == nil {
		return SizeMetrics{}, fmt.Errorf("store não inicializado")
	}
	return e.store.GetTotalSavings()
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

func (e *Engine) sendWebhook(job *Job, eventKind string) {
	if e.webhook == nil {
		return
	}
	jCopy := *job
	if eventKind == "completed" {
		jCopy.Status = StatusCompleted
	} else if eventKind == "failed" {
		if jCopy.Status != StatusRolledBack {
			jCopy.Status = StatusFailed
		}
	}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		if err := e.webhook.Send(&jCopy, eventKind); err != nil {
			e.log.Error("falha ao enviar webhook", "job_id", jCopy.ID, "event", eventKind, "error", err.Error())
		}
	}()
}

// Graceful encerra os workers aguardando conclusão da fila.
func (e *Engine) Graceful(ctx context.Context) {
	e.CloseCancellation()
	<-ctx.Done()
}
