// Command server é o binário HTTP do painel de controle web do CodecAny
// (Fase A: esqueleto + API somente-leitura + SSE + serving do frontend
// embutido via go:embed). Espelha cmd/cli/main.go::buildEngine — mesma
// composição de core.NewStore + core.NewRulesEngine + core.NewEngine com os
// adapters pkg/adapters/ffmpeg, sem nenhuma dependência nova de pkg/core →
// HTTP (ver docs/propostas_painel_controle.md, seção 1). É literalmente
// outro processo cliente do mesmo pkg/core.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/FM0Ura/codecany/pkg/adapters/ffmpeg"
	"github.com/FM0Ura/codecany/pkg/core"
	"github.com/FM0Ura/codecany/pkg/logger"
)

func main() {
	cfg, dirs, err := parseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}

	log, err := logger.New(logger.Config{Dir: cfg.LogDir, Level: cfg.LogLevel})
	if err != nil {
		fmt.Fprintln(os.Stderr, "logger:", err)
		os.Exit(1)
	}

	scanEvery, err := time.ParseDuration(cfg.ScanInterval)
	if err != nil {
		logger.Fatal(log, fmt.Errorf("scan-interval: %w", err))
	}

	events := make(chan core.JobEvent, 256)
	eng, err := buildEngine(cfg.DBPath, cfg.RulesPath, cfg.Workers, events, log)
	if err != nil {
		logger.Fatal(log, err)
	}
	eng.SetScanInterval(scanEvery)

	// -dir (Fase A, efêmero — nunca toca a tabela watched_dirs): mesmo
	// padrão de cmd/cli, só para já ter algo rodando/testável localmente
	// sem depender da API de diretórios.
	for _, d := range dirs {
		if err := eng.WatchDir(d); err != nil {
			log.Error("falha ao monitorar diretório", "dir", d, "error", err.Error())
		}
	}

	// Diretórios monitorados persistidos (Fase C): a partir de agora o
	// painel de controle é a fonte de verdade — carrega o que foi salvo via
	// API em execuções anteriores (tabela watched_dirs) e começa a
	// monitorá-los, além do -dir efêmero acima.
	persistedDirs, err := eng.ListWatchedDirs()
	if err != nil {
		log.Error("falha ao listar diretórios monitorados persistidos", "error", err.Error())
	}
	for _, d := range persistedDirs {
		if err := eng.WatchDir(d); err != nil {
			log.Error("falha ao monitorar diretório persistido", "dir", d, "error", err.Error())
		}
	}

	// Sinalização de shutdown: core.RunSignals (pkg/core/signals.go) em vez
	// do canal de os.Signal bruto compartilhado usado em cmd/cli — aqui o
	// sinal precisa acordar TRÊS consumidores independentes (abortar jobs em
	// andamento, cancelar o ctx dos workers e encerrar o http.Server), e um
	// chan buffered de tamanho 1 só entrega o valor a UM receptor. O ctx de
	// RunSignals resolve isso de graça: ctx.Done() é fechado (broadcast),
	// então múltiplos "<-ctx.Done()" independentes disparam corretamente.
	ctx, cancel := core.RunSignals(context.Background())
	defer cancel()

	go func() {
		<-ctx.Done()
		log.Warn("shutdown solicitado, abortando jobs")
		eng.CloseCancellation()
	}()

	eng.StartWatch()

	workersDone := make(chan struct{})
	go func() {
		defer close(workersDone)
		eng.OpenWorkers(ctx)
	}()

	broadcaster := NewBroadcaster()
	go broadcaster.Run(eng.Events())

	assets, err := newAssetHandler()
	if err != nil {
		logger.Fatal(log, fmt.Errorf("assets: %w", err))
	}

	app := &App{
		Engine:      eng,
		Broadcaster: broadcaster,
		Config:      cfg,
		StartedAt:   time.Now(),
		Assets:      assets,
		Log:         log,
	}

	httpServer := &http.Server{
		Addr:    cfg.Addr,
		Handler: app.Handler(),
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("servidor HTTP escutando", "addr", cfg.Addr)
		serverErr <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("servidor HTTP encerrado com erro", "error", err.Error())
		}
	}

	// Shutdown gracioso: para de aceitar novas conexões HTTP (inclusive SSE,
	// que se desliga sozinho ao ver r.Context().Done()), espera os workers
	// atuais abortarem (ctx já cancelado acima) e só então fecha
	// watcher+store via eng.Shutdown().
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("falha no shutdown gracioso do http.Server", "error", err.Error())
	}

	<-workersDone
	eng.Shutdown()
}

// buildEngine monta Store+RulesEngine+Engine com os adapters ffmpeg —
// mesmo padrão de cmd/cli/main.go::buildEngine.
func buildEngine(storePath, rulesPath string, workers int, events chan core.JobEvent, log *slog.Logger) (*core.Engine, error) {
	s, err := core.NewStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	re, err := core.NewRulesEngine(rulesPath)
	if err != nil {
		return nil, fmt.Errorf("rules: %w", err)
	}
	return core.NewEngine(core.EngineDeps{
		Store:    s,
		Rules:    re,
		Prober:   ffmpeg.NewProber(),
		Verifier: ffmpeg.NewVerifier(),
		Engines:  []core.TranscoderEngine{ffmpeg.NewTranscode()},
		Workers:  workers,
		Events:   events,
		Logger:   log,
	})
}
