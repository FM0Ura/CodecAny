// Command cli é o wrapper fino de linha de comando do CodecAny (seção 2/10).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/FM0Ura/codecany/pkg/adapters/ffmpeg"
	"github.com/FM0Ura/codecany/pkg/core"
	"github.com/FM0Ura/codecany/pkg/logger"
)

func main() {
	var dirs multiFlag
	var files multiFlag
	store := flag.String("db", "codecany.db", "arquivo SQLite (.db)")
	rules := flag.String("rules", "rules.yaml", "arquivo de regras YAML/JSON")
	workers := flag.Int("workers", 1, "número de workers")
	jsonLog := flag.Bool("json", false, "log estruturado em JSON")
	logDir := flag.String("log-dir", "logs", "pasta dos arquivos de log")
	logLevel := flag.String("log-level", "info", "nível de log (debug|info|warn|error)")
	scanInterval := flag.String("scan-interval", "1m", "intervalo da varredura periódica de fallback (ex.: 30s, 5m; 0 desliga)")
	flag.Var(&dirs, "dir", "diretório a monitorar (repita para vários)")
	flag.Var(&files, "file", "arquivo específico a processar uma vez (repita para vários)")
	flag.Parse()
	if len(dirs) == 0 && len(files) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	log, err := logger.New(logger.Config{
		Dir:         *logDir,
		JSONConsole: *jsonLog,
		Level:       *logLevel,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "logger:", err)
		os.Exit(1)
	}

	events := make(chan core.JobEvent, 256)
	go frameProgress(events, log)

	scanEvery, err := time.ParseDuration(*scanInterval)
	if err != nil {
		logger.Fatal(log, fmt.Errorf("scan-interval: %w", err))
	}

	eng, err := buildEngine(*store, *rules, *workers, events, log)
	if err != nil {
		logger.Fatal(log, err)
	}
	eng.SetScanInterval(scanEvery)

	if len(files) > 0 {
		runErr := eng.RunOnce(context.Background(), files)
		printSavingsReport(eng, log)
		eng.Shutdown()
		if runErr != nil {
			logger.Fatal(log, runErr)
		}
		return
	}

	for _, d := range dirs {
		if err := eng.WatchDir(d); err != nil {
			log.Error("falha ao monitorar diretório", "dir", d, "error", err.Error())
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Warn("shutdown solicitado, abortando jobs")
		eng.CloseCancellation()
	}()

	eng.StartWatch()
	eng.OpenWorkers(workCtx(sig))
	printSavingsReport(eng, log)
	eng.Shutdown()
}

// workCtx fornece um context.Context cancelado ao receber sinal de término.
func workCtx(sig chan os.Signal) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-sig
		cancel()
	}()
	return ctx
}

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

// frameProgress consome eventos nativos e exibe o progresso da conversão.
// Em terminal, desenha uma barra de progresso inline; caso contrário, registra
// via slog de forma esparsa (a cada 1%).
func frameProgress(events chan core.JobEvent, log *slog.Logger) {
	render := isTerminal(os.Stderr)
	bar := newProgressBar(os.Stderr)
	var lastPct = map[string]int{}
	for ev := range events {
		switch ev.Kind {
		case core.EventJobStart:
			if render {
				bar.endl(ev.FilePath)
			}
			log.Info("job iniciado", "job_id", ev.JobID, "path", ev.FilePath, "driver", ev.Driver)
		case core.EventJobProgress:
			pct := ev.Progress * 100
			if render {
				bar.update(ev.JobID, ev.FilePath, pct)
				continue
			}
			pctInt := int(pct)
			if pctInt >= 100 || pctInt-lastPct[ev.JobID] >= 1 {
				lastPct[ev.JobID] = pctInt
				log.Info("progresso", "job_id", ev.JobID, "path", ev.FilePath,
					"progress_pct", pctInt)
			}
		case core.EventJobComplete, core.EventJobError:
			if render {
				bar.endl(ev.FilePath) // finaliza a linha da barra
			}
		}
	}
}

// shortID reduz o UUID do job para exibição compacta.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// isTerminal informa se o writer é um terminal (fífio tty), habilitando a barra.
func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		stat, err := f.Stat()
		if err == nil {
			return (stat.Mode() & os.ModeCharDevice) != 0
		}
	}
	return false
}

// progressBar desenha uma barra de progresso reutilizando a linha atual do
// terminal via retorno de carro (\r).
type progressBar struct {
	w     io.Writer
	width int
}

func newProgressBar(w io.Writer) *progressBar {
	return &progressBar{w: w, width: 24}
}

// update redesenha a barra para o job em questão. O progresso é derivado do
// evento mais recente; múltiplos jobs concorrentes refletem o último recebido.
func (b *progressBar) update(jobID, path string, pct float64) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct * float64(b.width) / 100)
	bar := strings.Repeat("=", filled)
	if filled < b.width {
		bar += ">"
		bar += strings.Repeat(" ", b.width-filled-1)
	}
	line := fmt.Sprintf("\r[%s] %s %8.4f%% %s", shortID(jobID), filepath.Base(path), pct, bar)
	fmt.Fprint(b.w, line)
}

// endl encerra a linha atual da barra com um salto de linha.
func (b *progressBar) endl(path string) {
	fmt.Fprintf(b.w, "\r%s\n", strings.Repeat(" ", 8)) // limpa a linha
}

// multiFlag acumula valores repetidos de -dir.
type multiFlag []string

func (m *multiFlag) String() string { return fmt.Sprint(*m) }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func printSavingsReport(eng *core.Engine, log *slog.Logger) {
	metrics, err := eng.GetTotalSavings()
	if err != nil {
		log.Error("falha ao obter métricas de economia", "error", err.Error())
		return
	}
	if metrics.OriginalSizeBytes == 0 {
		return
	}

	origMB := float64(metrics.OriginalSizeBytes) / (1024 * 1024)
	convMB := float64(metrics.ConvertedSizeBytes) / (1024 * 1024)
	savedMB := float64(metrics.SavedBytes) / (1024 * 1024)

	fmt.Printf("\n==================================================\n")
	fmt.Printf("📊 RELATÓRIO CONSOLIDADO DE ECONOMIA DE ESPAÇO\n")
	fmt.Printf("==================================================\n")
	fmt.Printf("  Tamanho Original:  %.2f MB\n", origMB)
	fmt.Printf("  Tamanho Convertido: %.2f MB\n", convMB)
	fmt.Printf("  Espaço Economizado: %.2f MB\n", savedMB)
	fmt.Printf("  Taxa de Compressão:  %.2f%%\n", metrics.CompressionRatioPct)
	fmt.Printf("==================================================\n\n")
}
