package core

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
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
	rulesPath := writeRules(t, "version: 1\nglobal:\n  default_driver: mock\n  staging_dir: "+stage+"\n  space_saving:\n    min_saving_pct: 15\n  defaults:\n    video: { codec: hevc }\nrules:\n  - name: r\n    convert:\n      video: { codec: hevc }\n")

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
