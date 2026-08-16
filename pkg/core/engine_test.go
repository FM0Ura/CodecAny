package core

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupEngine monta um engine end-to-end com mocks e um único job na fila.
func setupEngine(t *testing.T, origSize, outSize int64) (*Engine, chan JobEvent, string, string) {
	return setupEngineWithVerifier(t, origSize, outSize, nil)
}

func setupEngineWithVerifier(t *testing.T, origSize, outSize int64, verifier MediaVerifier) (*Engine, chan JobEvent, string, string) {
	t.Helper()
	base := t.TempDir()
	stage := filepath.Join(base, "staging")
	mediaPath := filepath.Join(base, "movie.mkv")
	writeFileSize(t, mediaPath, origSize)

	// regras com staging local e driver mock
	rulesPath := writeRules(t, "version: 1\nglobal:\n  default_driver: mock\n  staging_dir: "+stage+"\n  space_saving:\n    min_saving_pct: 15\n  defaults:\n    video: { codec: hevc }\n    auto_approve: true\nrules:\n  - name: r\n    convert:\n      video: { codec: hevc }\n")

	store, err := NewStore(filepath.Join(base, "codecany.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	re, err := NewRulesEngine(rulesPath)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan JobEvent, 64)
	eng, err := NewEngine(EngineDeps{
		Store:    store,
		Rules:    re,
		Prober:   mockProber{info: MediaInfo{Container: "mkv", VideoCodec: "h264"}},
		Verifier: verifier,
		Engines:  []TranscoderEngine{&mockTranscoder{outputSize: outSize}},
		Workers:  1,
		Events:   events,
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return eng, events, mediaPath, stage
}

func TestEngineCompletesJob(t *testing.T) {
	eng, events, mediaPath, stage := setupEngine(t, 1000, 500) // 50% economia
	eng.HandleDiscovered(mediaPath)

	sigDone := make(chan struct{})
	go func() {
		defer close(sigDone)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			time.Sleep(2 * time.Second)
			cancel()
		}()
		eng.OpenWorkers(ctx)
	}()

	evs := drainEvents(events, 3*time.Second)
	<-sigDone

	var okComplete bool
	for _, ev := range evs {
		if ev.Kind == EventJobComplete && ev.Success {
			okComplete = true
		}
	}
	if !okComplete {
		t.Fatalf("esperava OnJobComplete success; recebidos: %+v", evs)
	}

	// original foi substituído pelo output (500 bytes)
	fi, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 500 {
		t.Errorf("tamanho final esperado 500, obteve %d", fi.Size())
	}
	// zero residue: staging do job purgada
	if _, err := os.Stat(filepath.Join(stage, "movie")); err == nil {
		t.Errorf("staging do job deveria ser purgada após sucesso")
	}
}

func TestEngineRollsBackInsufficientGain(t *testing.T) {
	eng, events, mediaPath, stage := setupEngine(t, 1000, 950) // 5% < 15%
	eng.HandleDiscovered(mediaPath)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			time.Sleep(2 * time.Second)
			cancel()
		}()
		eng.OpenWorkers(ctx)
	}()
	evs := drainEvents(events, 3*time.Second)
	<-done

	var rolledBack bool
	for _, ev := range evs {
		if ev.Kind == EventJobComplete && !ev.Success {
			rolledBack = true
		}
	}
	if !rolledBack {
		t.Fatalf("esperava rollback; recebidos: %+v", evs)
	}
	// original preservado
	fi, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 1000 {
		t.Errorf("original deveria permanecer 1000, obteve %d", fi.Size())
	}
	if _, err := os.Stat(filepath.Join(stage, "movie")); err == nil {
		t.Errorf("staging do job deveria ser purgada após rollback")
	}
}

func TestEngineRunOnceDrainsQueue(t *testing.T) {
	eng, events, mediaPath, _ := setupEngine(t, 1000, 500)

	done := make(chan error, 1)
	go func() { done <- eng.RunOnce(context.Background(), []string{mediaPath}) }()

	evs := drainEvents(events, 3*time.Second)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunOnce: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunOnce não retornou dentro do tempo limite")
	}

	var okComplete bool
	for _, ev := range evs {
		if ev.Kind == EventJobComplete && ev.Success {
			okComplete = true
		}
	}
	if !okComplete {
		t.Fatalf("esperava OnJobComplete success; recebidos: %+v", evs)
	}
	if n, err := eng.store.PendingJobCount(); err != nil || n != 0 {
		t.Errorf("fila deveria estar vazia após RunOnce, pendentes=%d err=%v", n, err)
	}
}

func TestEngineRunOnceReportsSkippedFiles(t *testing.T) {
	eng, events, mediaPath, _ := setupEngine(t, 1000, 500)

	missing := filepath.Join(t.TempDir(), "missing.mkv")
	done := make(chan error, 1)
	go func() {
		done <- eng.RunOnce(context.Background(), []string{missing, mediaPath})
	}()

	evs := drainEvents(events, 3*time.Second)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("RunOnce deveria reportar arquivo não processado")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunOnce não retornou dentro do tempo limite")
	}

	var okComplete bool
	for _, ev := range evs {
		if ev.Kind == EventJobComplete && ev.Success {
			okComplete = true
		}
	}
	if !okComplete {
		t.Fatalf("arquivo válido ainda deveria ser processado; recebidos: %+v", evs)
	}
}

func TestEngineFailsWhenVerificationRejectsOutput(t *testing.T) {
	eng, events, mediaPath, stage := setupEngineWithVerifier(t, 1000, 500, mockVerifier{
		err: errors.New("decode: broken file"),
	})
	eng.HandleDiscovered(mediaPath)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			time.Sleep(2 * time.Second)
			cancel()
		}()
		eng.OpenWorkers(ctx)
	}()
	evs := drainEvents(events, 3*time.Second)
	<-done

	var errored bool
	for _, ev := range evs {
		if ev.Kind == EventJobError {
			errored = true
		}
	}
	if !errored {
		t.Fatalf("esperava EventJobError; recebidos: %+v", evs)
	}
	// original preservado: verificação falhou ⇒ nenhuma troca
	fi, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 1000 {
		t.Errorf("original deveria permanecer 1000, obteve %d", fi.Size())
	}
	if _, err := os.Stat(filepath.Join(stage, "movie")); err == nil {
		t.Errorf("staging do job deveria ser purgada após falha de verificação")
	}
}

func TestEngineCompletesWithPassingVerification(t *testing.T) {
	eng, events, mediaPath, _ := setupEngineWithVerifier(t, 1000, 500, mockVerifier{err: nil})
	eng.HandleDiscovered(mediaPath)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			time.Sleep(2 * time.Second)
			cancel()
		}()
		eng.OpenWorkers(ctx)
	}()
	evs := drainEvents(events, 3*time.Second)
	<-done

	var ok bool
	for _, ev := range evs {
		if ev.Kind == EventJobComplete && ev.Success {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("esperava OnJobComplete success; recebidos: %+v", evs)
	}
}

// setupEngineAwaitingApproval monta um engine com auto_approve=false (default
// da suíte, já que setupEngine/setupEngineWithVerifier passam a exigir
// auto_approve: true explicitamente nas rules) e devolve store/staging/rules
// separados para permitir a reconstrução de uma segunda instância de Engine
// sobre o mesmo .db/staging (ver TestEngineApproveJobFromFreshEngineInstance).
func setupEngineAwaitingApproval(t *testing.T, base string, origSize, outSize int64) (eng *Engine, events chan JobEvent, mediaPath, stage, dbPath, rulesPath string) {
	t.Helper()
	stage = filepath.Join(base, "staging")
	mediaPath = filepath.Join(base, "movie.mkv")
	writeFileSize(t, mediaPath, origSize)

	rulesPath = writeRules(t, "version: 1\nglobal:\n  default_driver: mock\n  staging_dir: "+stage+"\n  space_saving:\n    min_saving_pct: 15\n  defaults:\n    video: { codec: hevc }\nrules:\n  - name: r\n    convert:\n      video: { codec: hevc }\n")

	dbPath = filepath.Join(base, "codecany.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	re, err := NewRulesEngine(rulesPath)
	if err != nil {
		t.Fatal(err)
	}
	events = make(chan JobEvent, 64)
	eng, err = NewEngine(EngineDeps{
		Store:   store,
		Rules:   re,
		Prober:  mockProber{info: MediaInfo{Container: "mkv", VideoCodec: "h264"}},
		Engines: []TranscoderEngine{&mockTranscoder{outputSize: outSize}},
		Workers: 1,
		Events:  events,
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return eng, events, mediaPath, stage, dbPath, rulesPath
}

// runToCompletion enfileira mediaPath e drena o engine até a fila esvaziar,
// devolvendo os eventos emitidos.
func runToCompletion(t *testing.T, eng *Engine, events chan JobEvent, mediaPath string) []JobEvent {
	t.Helper()
	eng.HandleDiscovered(mediaPath)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			time.Sleep(2 * time.Second)
			cancel()
		}()
		eng.OpenWorkers(ctx)
	}()
	evs := drainEvents(events, 3*time.Second)
	<-done
	return evs
}

// TestEngineAwaitsApprovalWhenAutoApproveFalse cobre o caminho central da
// Fase 1: sem auto_approve, um job com integridade OK e economia suficiente
// pausa em AWAITING_APPROVAL em vez de commitar — evento emitido com métricas,
// original intocado, output presente em staging.
func TestEngineAwaitsApprovalWhenAutoApproveFalse(t *testing.T) {
	base := t.TempDir()
	eng, events, mediaPath, stage, _, _ := setupEngineAwaitingApproval(t, base, 1000, 500)
	evs := runToCompletion(t, eng, events, mediaPath)

	var awaiting *JobEvent
	for i, ev := range evs {
		if ev.Kind == EventJobAwaitingApproval {
			awaiting = &evs[i]
		}
	}
	if awaiting == nil {
		t.Fatalf("esperava EventJobAwaitingApproval; recebidos: %+v", evs)
	}
	if awaiting.Metrics.SavedBytes != 500 || awaiting.Metrics.OriginalSizeBytes != 1000 {
		t.Errorf("métricas do evento incorretas: %+v", awaiting.Metrics)
	}
	if awaiting.TargetCodec != "hevc" {
		t.Errorf("TargetCodec esperado hevc, obteve %q", awaiting.TargetCodec)
	}

	// original intocado
	fi, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 1000 {
		t.Errorf("original deveria permanecer 1000, obteve %d", fi.Size())
	}
	// output em staging (não purgado)
	if _, err := os.Stat(filepath.Join(stage, "movie", "output.mkv")); err != nil {
		t.Errorf("esperava output em staging preservado: %v", err)
	}

	job, err := eng.store.FindByPath(mediaPath)
	if err != nil || job == nil {
		t.Fatalf("esperava job persistido: %v", err)
	}
	if job.Status != StatusAwaitingApproval {
		t.Errorf("status persistido = %v, esperado %v", job.Status, StatusAwaitingApproval)
	}
	if job.SizeMetrics.SavedBytes != 500 {
		t.Errorf("métricas persistidas incorretas: %+v", job.SizeMetrics)
	}
}

// TestEngineApproveJobCommits confirma que ApproveJob comita o output em
// staging no lugar do original e marca COMPLETED.
func TestEngineApproveJobCommits(t *testing.T) {
	base := t.TempDir()
	eng, events, mediaPath, stage, _, _ := setupEngineAwaitingApproval(t, base, 1000, 500)
	runToCompletion(t, eng, events, mediaPath)

	job, err := eng.store.FindByPath(mediaPath)
	if err != nil || job == nil {
		t.Fatalf("esperava job AWAITING_APPROVAL: %v", err)
	}

	if err := eng.ApproveJob(job.ID); err != nil {
		t.Fatalf("ApproveJob falhou: %v", err)
	}

	fi, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 500 {
		t.Errorf("esperava arquivo trocado (500 bytes), obteve %d", fi.Size())
	}
	if _, err := os.Stat(filepath.Join(stage, "movie")); err == nil {
		t.Errorf("staging deveria ser purgada após approve")
	}

	got, err := eng.store.FindByID(job.ID)
	if err != nil || got == nil {
		t.Fatalf("job deveria continuar no store: %v", err)
	}
	if got.Status != StatusCompleted {
		t.Errorf("status esperado COMPLETED, obteve %v", got.Status)
	}
}

// TestEngineRejectJobAborts confirma que RejectJob descarta o staging e
// preserva o original, marcando ROLLED_BACK.
func TestEngineRejectJobAborts(t *testing.T) {
	base := t.TempDir()
	eng, events, mediaPath, stage, _, _ := setupEngineAwaitingApproval(t, base, 1000, 500)
	runToCompletion(t, eng, events, mediaPath)

	job, err := eng.store.FindByPath(mediaPath)
	if err != nil || job == nil {
		t.Fatalf("esperava job AWAITING_APPROVAL: %v", err)
	}

	if err := eng.RejectJob(job.ID); err != nil {
		t.Fatalf("RejectJob falhou: %v", err)
	}

	fi, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 1000 {
		t.Errorf("esperava original preservado (1000 bytes), obteve %d", fi.Size())
	}
	if _, err := os.Stat(filepath.Join(stage, "movie")); err == nil {
		t.Errorf("staging deveria ser purgada após reject")
	}

	got, err := eng.store.FindByID(job.ID)
	if err != nil || got == nil {
		t.Fatalf("job deveria continuar no store: %v", err)
	}
	if got.Status != StatusRolledBack {
		t.Errorf("status esperado ROLLED_BACK, obteve %v", got.Status)
	}
}

// TestEngineApproveJobFromFreshEngineInstance prova o ponto-chave do design:
// ApproveJob reconstrói o Cleanup de forma 100% determinística a partir de
// job.Path/staging/container, então funciona a partir de uma SEGUNDA
// instância de Engine sobre o mesmo .db/staging — o cenário real de
// `-approve <id>` rodando num processo CLI novo, separado do que criou o job.
func TestEngineApproveJobFromFreshEngineInstance(t *testing.T) {
	base := t.TempDir()
	eng1, events, mediaPath, stage, dbPath, rulesPath := setupEngineAwaitingApproval(t, base, 1000, 500)
	runToCompletion(t, eng1, events, mediaPath)

	job, err := eng1.store.FindByPath(mediaPath)
	if err != nil || job == nil {
		t.Fatalf("esperava job AWAITING_APPROVAL: %v", err)
	}
	eng1.Shutdown() // fecha o store da primeira instância (simula fim do processo CLI)

	// Segunda instância "processo novo": abre o MESMO .db/staging do zero.
	store2, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store2.Close() })
	re2, err := NewRulesEngine(rulesPath)
	if err != nil {
		t.Fatal(err)
	}
	eng2, err := NewEngine(EngineDeps{
		Store:   store2,
		Rules:   re2,
		Prober:  mockProber{info: MediaInfo{Container: "mkv", VideoCodec: "h264"}},
		Engines: []TranscoderEngine{&mockTranscoder{outputSize: 500}},
		Workers: 1,
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := eng2.ApproveJob(job.ID); err != nil {
		t.Fatalf("ApproveJob (segunda instância) falhou: %v", err)
	}

	fi, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != 500 {
		t.Errorf("esperava arquivo trocado (500 bytes), obteve %d", fi.Size())
	}
	if _, err := os.Stat(filepath.Join(stage, "movie")); err == nil {
		t.Errorf("staging deveria ser purgada após approve na segunda instância")
	}

	got, err := store2.FindByID(job.ID)
	if err != nil || got == nil {
		t.Fatalf("job deveria continuar no store: %v", err)
	}
	if got.Status != StatusCompleted {
		t.Errorf("status esperado COMPLETED, obteve %v", got.Status)
	}
}

// setupHWAccelEngine monta um engine com um controllableTranscoder registrado
// sob o driver "mock" e dois jobs QUEUED persistidos (movieA/movieB), prontos
// para serem disparados diretamente via e.runJob (em vez de OpenWorkers) —
// isso evita depender do pool de workers/fila (Store.NextPendingJob hoje é um
// SELECT simples sem claim atômico; disparar via workerLoop com Workers>1
// arrisca duas goroutines pegarem o mesmo job QUEUED, o que é uma
// pré-existência fora do escopo desta fase e só tornaria o teste flaky).
// Chamar runJob diretamente com dois *Job distintos testa exatamente a lógica
// do semáforo de hwaccel de forma determinística.
func setupHWAccelEngine(t *testing.T, rulesYAML string) (eng *Engine, ct *controllableTranscoder, jobA, jobB *Job) {
	t.Helper()
	base := t.TempDir()
	stage := filepath.Join(base, "staging")
	mediaA := filepath.Join(base, "movieA.mkv")
	mediaB := filepath.Join(base, "movieB.mkv")
	writeFileSize(t, mediaA, 1000)
	writeFileSize(t, mediaB, 1000)

	rulesPath := writeRules(t, strings.ReplaceAll(rulesYAML, "__STAGE__", stage))

	store, err := NewStore(filepath.Join(base, "codecany.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	re, err := NewRulesEngine(rulesPath)
	if err != nil {
		t.Fatal(err)
	}

	ct = &controllableTranscoder{
		outputSize:    500,
		release:       make(chan struct{}),
		startedNotify: make(chan struct{}, 2),
	}
	events := make(chan JobEvent, 64)
	eng, err = NewEngine(EngineDeps{
		Store:   store,
		Rules:   re,
		Prober:  mockProber{info: MediaInfo{Container: "mkv", VideoCodec: "h264"}},
		Engines: []TranscoderEngine{ct},
		Workers: 2,
		Events:  events,
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !eng.HandleDiscovered(mediaA) {
		t.Fatal("esperava job A enfileirado")
	}
	if !eng.HandleDiscovered(mediaB) {
		t.Fatal("esperava job B enfileirado")
	}
	jobA, err = store.FindByPath(mediaA)
	if err != nil || jobA == nil {
		t.Fatalf("esperava job A persistido: %v", err)
	}
	jobB, err = store.FindByPath(mediaB)
	if err != nil || jobB == nil {
		t.Fatalf("esperava job B persistido: %v", err)
	}
	return eng, ct, jobA, jobB
}

// TestEngineHWAccelLimitBlocksConcurrentTranscodes prova que
// global.hwaccel_limits realmente serializa transcodificações concorrentes
// que usam o mesmo vendor de hwaccel: com o limite configurado em 1, um
// segundo job só chama Transcode() depois que o primeiro libera o slot do
// semáforo (fim do loop de progresso em runJob).
func TestEngineHWAccelLimitBlocksConcurrentTranscodes(t *testing.T) {
	eng, ct, jobA, jobB := setupHWAccelEngine(t, "version: 1\nglobal:\n  default_driver: mock\n  staging_dir: __STAGE__\n  space_saving:\n    min_saving_pct: 15\n  hwaccel_limits:\n    mock: 1\n  defaults:\n    video: { codec: hevc }\n    auto_approve: true\nrules:\n  - name: r\n    convert:\n      video: { codec: hevc, hwaccel: mock }\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneA := make(chan struct{})
	doneB := make(chan struct{})
	go func() { defer close(doneA); eng.runJob(ctx, jobA) }()
	go func() { defer close(doneB); eng.runJob(ctx, jobB) }()

	// Um dos dois jobs entra em Transcode primeiro.
	select {
	case <-ct.startedNotify:
	case <-time.After(2 * time.Second):
		t.Fatal("nenhum job chamou Transcode dentro do tempo limite")
	}

	// O outro NÃO deve chamar Transcode enquanto o primeiro segura o único
	// slot do semáforo (hwaccel_limits: {mock: 1}).
	select {
	case <-ct.startedNotify:
		t.Fatal("segundo job chamou Transcode antes do primeiro liberar o slot de hwaccel")
	case <-time.After(300 * time.Millisecond):
		// esperado: nada chegou ainda
	}

	// Libera o primeiro job (fecha seu canal de progresso) — o slot do
	// semáforo é liberado assim que o loop de progresso de runJob termina,
	// não depois de TESTING/Commit.
	select {
	case ct.release <- struct{}{}:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout ao liberar o primeiro job")
	}

	// Agora o segundo job deve conseguir adquirir o slot e chamar Transcode.
	select {
	case <-ct.startedNotify:
	case <-time.After(2 * time.Second):
		t.Fatal("segundo job não chamou Transcode após o primeiro liberar o slot de hwaccel")
	}

	// Libera o segundo job para os dois runJob concluírem.
	select {
	case ct.release <- struct{}{}:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout ao liberar o segundo job")
	}

	<-doneA
	<-doneB
}

// TestEngineNoHWAccelLimitAllowsConcurrentTranscodes confirma que uma regra
// SEM hwaccel configurado (ou sem limite correspondente em
// global.hwaccel_limits) não é afetada por nenhum semáforo — comportamento
// idêntico ao anterior à Fase 4: os dois jobs concorrentes chamam Transcode
// livremente, sem que um precise esperar o outro liberar.
func TestEngineNoHWAccelLimitAllowsConcurrentTranscodes(t *testing.T) {
	// Sem hwaccel na regra e sem hwaccel_limits configurado.
	eng, ct, jobA, jobB := setupHWAccelEngine(t, "version: 1\nglobal:\n  default_driver: mock\n  staging_dir: __STAGE__\n  space_saving:\n    min_saving_pct: 15\n  defaults:\n    video: { codec: hevc }\n    auto_approve: true\nrules:\n  - name: r\n    convert:\n      video: { codec: hevc }\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneA := make(chan struct{})
	doneB := make(chan struct{})
	go func() { defer close(doneA); eng.runJob(ctx, jobA) }()
	go func() { defer close(doneB); eng.runJob(ctx, jobB) }()

	// Ambos os jobs devem conseguir chamar Transcode concorrentemente, sem
	// que um precise esperar o outro liberar (nenhum semáforo em jogo).
	for i := 0; i < 2; i++ {
		select {
		case <-ct.startedNotify:
		case <-time.After(2 * time.Second):
			t.Fatalf("job %d não chamou Transcode dentro do tempo limite (deveria rodar livremente sem limite de hwaccel)", i)
		}
	}

	// Libera ambos para os runJob concluírem.
	for i := 0; i < 2; i++ {
		select {
		case ct.release <- struct{}{}:
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout ao liberar job %d", i)
		}
	}

	<-doneA
	<-doneB
}

func TestEngineWebhooks(t *testing.T) {
	called := make(chan string, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	base := t.TempDir()
	stage := filepath.Join(base, "staging")
	mediaPath := filepath.Join(base, "movie.mkv")
	writeFileSize(t, mediaPath, 1000)

	// Regras com staging local e webhook configurado
	rulesContent := `
version: 1
global:
  default_driver: mock
  staging_dir: ` + stage + `
  space_saving:
    min_saving_pct: 15
  defaults:
    video: { codec: hevc }
    auto_approve: true
  notifications:
    webhook_url: ` + ts.URL + `
    events: [completed]
rules:
  - name: r
    convert:
      video: { codec: hevc }
`
	rulesPath := writeRules(t, rulesContent)

	store, err := NewStore(filepath.Join(base, "codecany.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	re, err := NewRulesEngine(rulesPath)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan JobEvent, 64)
	eng, err := NewEngine(EngineDeps{
		Store:   store,
		Rules:   re,
		Prober:  mockProber{info: MediaInfo{Container: "mkv", VideoCodec: "h264"}},
		Engines: []TranscoderEngine{&mockTranscoder{outputSize: 500}}, // 50% economia
		Workers: 1,
		Events:  events,
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}

	eng.HandleDiscovered(mediaPath)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			time.Sleep(2 * time.Second)
			cancel()
		}()
		eng.OpenWorkers(ctx)
	}()

	drainEvents(events, 3*time.Second)
	<-done

	select {
	case <-called:
		// Sucesso!
	case <-time.After(3 * time.Second):
		t.Errorf("webhook não foi acionado pelo Engine")
	}
}

// TestEngineUpdatesPathOnContainerChange cobre a regressão em que, após uma
// conversão que muda o container (extensão) do arquivo, job.Path — e tudo
// que dele deriva (JobEvent.FilePath, o registro no store, logs) — continuava
// apontando para o caminho antigo, que deixa de existir em disco após o
// Commit atômico do Cleanup.
func TestEngineUpdatesPathOnContainerChange(t *testing.T) {
	base := t.TempDir()
	stage := filepath.Join(base, "staging")
	mediaPath := filepath.Join(base, "movie.mp4")
	writeFileSize(t, mediaPath, 1000)

	rulesPath := writeRules(t, "version: 1\nglobal:\n  default_driver: mock\n  staging_dir: "+stage+"\n  space_saving:\n    min_saving_pct: 15\n  defaults:\n    video: { codec: hevc }\n    auto_approve: true\nrules:\n  - name: r\n    convert:\n      video: { codec: hevc }\n      container: mkv\n")

	store, err := NewStore(filepath.Join(base, "codecany.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	re, err := NewRulesEngine(rulesPath)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan JobEvent, 64)
	eng, err := NewEngine(EngineDeps{
		Store:   store,
		Rules:   re,
		Prober:  mockProber{info: MediaInfo{Container: "mp4", VideoCodec: "h264"}},
		Engines: []TranscoderEngine{&mockTranscoder{outputSize: 500}}, // 50% economia
		Workers: 1,
		Events:  events,
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}

	eng.HandleDiscovered(mediaPath)
	wantPath := filepath.Join(base, "movie.mkv")

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			time.Sleep(2 * time.Second)
			cancel()
		}()
		eng.OpenWorkers(ctx)
	}()
	evs := drainEvents(events, 3*time.Second)
	<-done

	var complete *JobEvent
	for i, ev := range evs {
		if ev.Kind == EventJobComplete && ev.Success {
			complete = &evs[i]
		}
	}
	if complete == nil {
		t.Fatalf("esperava EventJobComplete success; recebidos: %+v", evs)
	}
	if complete.FilePath != wantPath {
		t.Errorf("JobEvent.FilePath = %q, esperado %q (novo caminho após troca de container)", complete.FilePath, wantPath)
	}

	// Arquivo final existe com a nova extensão; o antigo (.mp4) não existe mais.
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("esperava %s presente: %v", wantPath, err)
	}
	if _, err := os.Stat(mediaPath); !os.IsNotExist(err) {
		t.Errorf("esperava %s ausente após troca de container, err=%v", mediaPath, err)
	}

	// O registro no store também deve refletir o novo caminho.
	job, err := store.FindByPath(wantPath)
	if err != nil || job == nil {
		t.Fatalf("esperava job no store sob o novo caminho: %v", err)
	}
	if job.Status != StatusCompleted {
		t.Errorf("status do job = %v, esperado %v", job.Status, StatusCompleted)
	}
	if stale, err := store.FindByPath(mediaPath); err == nil && stale != nil {
		t.Errorf("não esperava registro remanescente sob o caminho antigo: %+v", stale)
	}
}

// TestEngineHWAccelStatus prova que HWAccelStatus reflete corretamente
// Limit (capacidade do semáforo) e InUse (slots ocupados) enquanto um job
// está em voo, usando o mesmo controllableTranscoder/release de
// TestEngineHWAccelLimitBlocksConcurrentTranscodes para segurar o transcode
// em andamento até o teste confirmar o estado esperado.
func TestEngineHWAccelStatus(t *testing.T) {
	eng, ct, jobA, jobB := setupHWAccelEngine(t, "version: 1\nglobal:\n  default_driver: mock\n  staging_dir: __STAGE__\n  space_saving:\n    min_saving_pct: 15\n  hwaccel_limits:\n    mock: 2\n  defaults:\n    video: { codec: hevc }\n    auto_approve: true\nrules:\n  - name: r\n    convert:\n      video: { codec: hevc, hwaccel: mock }\n")
	_ = jobB

	// Antes de qualquer job rodar, o vendor "mock" aparece com o limite
	// configurado e nenhum slot em uso.
	status := eng.HWAccelStatus()
	if len(status) != 1 {
		t.Fatalf("esperava 1 vendor na lista, obteve %d: %+v", len(status), status)
	}
	if status[0].Vendor != "mock" || status[0].Limit != 2 || status[0].InUse != 0 {
		t.Errorf("status inicial inesperado: %+v", status[0])
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	doneA := make(chan struct{})
	go func() { defer close(doneA); eng.runJob(ctx, jobA) }()

	select {
	case <-ct.startedNotify:
	case <-time.After(2 * time.Second):
		t.Fatal("job A não chamou Transcode dentro do tempo limite")
	}

	// Com o job A em voo, InUse deve refletir 1 slot ocupado do limite de 2.
	status = eng.HWAccelStatus()
	if len(status) != 1 {
		t.Fatalf("esperava 1 vendor na lista, obteve %d: %+v", len(status), status)
	}
	if status[0].Vendor != "mock" || status[0].Limit != 2 || status[0].InUse != 1 {
		t.Errorf("status com job em voo inesperado: %+v", status[0])
	}

	select {
	case ct.release <- struct{}{}:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout ao liberar o job A")
	}
	<-doneA

	// Job liberado: o slot volta a ficar livre.
	status = eng.HWAccelStatus()
	if status[0].InUse != 0 {
		t.Errorf("esperava InUse=0 após o job liberar o slot, obteve %+v", status[0])
	}
}

// TestEngineHWAccelStatusEmptyWithoutLimits confirma que vendors sem limite
// configurado em global.hwaccel_limits não aparecem na lista.
func TestEngineHWAccelStatusEmptyWithoutLimits(t *testing.T) {
	eng, _, _, _ := setupHWAccelEngine(t, "version: 1\nglobal:\n  default_driver: mock\n  staging_dir: __STAGE__\n  space_saving:\n    min_saving_pct: 15\n  defaults:\n    video: { codec: hevc }\n    auto_approve: true\nrules:\n  - name: r\n    convert:\n      video: { codec: hevc }\n")
	if status := eng.HWAccelStatus(); len(status) != 0 {
		t.Errorf("esperava lista vazia sem hwaccel_limits configurado, obteve %+v", status)
	}
}

// TestEngineGetJob cobre o wrapper trivial de Store.FindByID: job existente
// retorna o job certo, inexistente retorna (nil, nil) sem erro.
func TestEngineGetJob(t *testing.T) {
	eng, _, mediaPath, _ := setupEngine(t, 1000, 500)
	eng.HandleDiscovered(mediaPath)

	want, err := eng.store.FindByPath(mediaPath)
	if err != nil || want == nil {
		t.Fatalf("esperava job persistido: %v", err)
	}

	got, err := eng.GetJob(want.ID)
	if err != nil {
		t.Fatalf("GetJob falhou: %v", err)
	}
	if got == nil || got.ID != want.ID {
		t.Fatalf("GetJob(%s) = %+v, esperado job %+v", want.ID, got, want)
	}

	missing, err := eng.GetJob("não-existe")
	if err != nil {
		t.Fatalf("GetJob de id inexistente não deveria retornar erro: %v", err)
	}
	if missing != nil {
		t.Errorf("esperava nil para id inexistente, obteve %+v", missing)
	}
}

// TestEngineCountJobsByStatus confirma que o wrapper do Engine delega
// corretamente para Store.CountByStatus.
func TestEngineCountJobsByStatus(t *testing.T) {
	eng, events, mediaPath, _ := setupEngine(t, 1000, 500) // 50% economia -> COMPLETED
	runToCompletion(t, eng, events, mediaPath)

	counts, err := eng.CountJobsByStatus()
	if err != nil {
		t.Fatalf("CountJobsByStatus falhou: %v", err)
	}
	if counts[StatusCompleted] != 1 {
		t.Errorf("esperava 1 job COMPLETED, obteve %+v", counts)
	}
	if counts[StatusQueued] != 0 {
		t.Errorf("não esperava jobs QUEUED, obteve %+v", counts)
	}
}

// TestEngineRollbackPersistsMetrics é o teste de regressão do item 14: força
// um cenário de economia insuficiente (mesmo setup de
// TestEngineRollsBackInsufficientGain) e confirma que, após runJob, o Job
// RECARREGADO DO STORE (não a cópia em memória) tem SizeMetrics não-zerado —
// antes do fix, só job.SizeMetrics (em memória) era atualizado no ramo de
// rollback; e.store.UpdateMetrics nunca era chamado, então
// original_size/converted_size/saved_bytes ficavam zerados no banco.
func TestEngineRollbackPersistsMetrics(t *testing.T) {
	eng, events, mediaPath, _ := setupEngine(t, 1000, 950) // 5% < 15% (min_saving_pct)
	runToCompletion(t, eng, events, mediaPath)

	job, err := eng.store.FindByPath(mediaPath)
	if err != nil || job == nil {
		t.Fatalf("esperava job persistido: %v", err)
	}
	if job.Status != StatusRolledBack {
		t.Fatalf("esperava status ROLLED_BACK, obteve %v", job.Status)
	}
	if job.SizeMetrics.OriginalSizeBytes != 1000 {
		t.Errorf("SizeMetrics.OriginalSizeBytes esperado 1000, obteve %d (métricas não persistidas no rollback)", job.SizeMetrics.OriginalSizeBytes)
	}
	if job.SizeMetrics.ConvertedSizeBytes != 950 {
		t.Errorf("SizeMetrics.ConvertedSizeBytes esperado 950, obteve %d (métricas não persistidas no rollback)", job.SizeMetrics.ConvertedSizeBytes)
	}
	if job.SizeMetrics.SavedBytes != 50 {
		t.Errorf("SizeMetrics.SavedBytes esperado 50, obteve %d (métricas não persistidas no rollback)", job.SizeMetrics.SavedBytes)
	}
}
