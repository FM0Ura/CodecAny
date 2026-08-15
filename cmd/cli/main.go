// Command cli é o wrapper fino de linha de comando do CodecAny (seção 2/10).
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
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

	// Comandos administrativos de staging/aprovação (Fase 1 do v1.2) e de
	// histórico filtrável (-history, Fase 6): não sobem worker/watcher, só
	// consultam/alteram o Store existente. Ver runManagementCommand.
	listStaged := flag.Bool("list-staged", false, "lista jobs aguardando aprovação manual e sai")
	approveID := flag.String("approve", "", "aprova o job com o ID informado e sai")
	approveAll := flag.Bool("approve-all", false, "aprova todos os jobs aguardando aprovação e sai")
	rejectID := flag.String("reject", "", "rejeita o job com o ID informado e sai")
	rejectAll := flag.Bool("reject-all", false, "rejeita todos os jobs aguardando aprovação e sai")

	// -history (Fase 6): relatório filtrável do histórico de jobs, mesmo
	// dispatcher administrativo da Fase 1 (não sobe worker/watcher).
	history := flag.Bool("history", false, "lista o histórico de jobs (com -status/-since) e sai")
	historyStatus := flag.String("status", "", "filtra -history por status (ex.: COMPLETED, FAILED)")
	historySince := flag.String("since", "", "filtra -history por janela de tempo (ex.: 24h, 30m)")
	flag.Parse()

	if *listStaged || *approveID != "" || *approveAll || *rejectID != "" || *rejectAll || *history {
		if *history && *historySince != "" {
			if _, err := time.ParseDuration(*historySince); err != nil {
				fmt.Fprintln(os.Stderr, "erro: -since inválido:", err)
				os.Exit(2)
			}
		}
		os.Exit(runManagementCommand(managementArgs{
			storePath:     *store,
			rulesPath:     *rules,
			listStaged:    *listStaged,
			approveID:     *approveID,
			approveAll:    *approveAll,
			rejectID:      *rejectID,
			rejectAll:     *rejectAll,
			history:       *history,
			historyStatus: *historyStatus,
			historySince:  *historySince,
		}, os.Stdout))
	}

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

	scanEvery, err := time.ParseDuration(*scanInterval)
	if err != nil {
		logger.Fatal(log, fmt.Errorf("scan-interval: %w", err))
	}

	eng, err := buildEngine(*store, *rules, *workers, events, log)
	if err != nil {
		logger.Fatal(log, err)
	}
	eng.SetScanInterval(scanEvery)

	// approvalsWG conta os prompts interativos de aprovação em andamento
	// (disparados por frameProgress em goroutines dedicadas). Sem esperar por
	// eles, eng.Shutdown() poderia fechar o Store antes do usuário responder
	// ao prompt y/N — ver waitApprovals.
	var approvalsWG sync.WaitGroup
	go frameProgress(events, log, eng, &approvalsWG)

	if len(files) > 0 {
		runErr := eng.RunOnce(context.Background(), files)
		printSavingsReport(eng, log)
		// Modo -file: sem timeout. É o cenário síncrono central da Fase 1 —
		// esperar a decisão do usuário é o comportamento correto aqui.
		waitApprovals(&approvalsWG, 0, log)
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
	// Modo daemon/watch: timeout curto (mesmo padrão de Engine.waitWebhooks)
	// para não travar o encerramento por um terminal de prompt abandonado.
	waitApprovals(&approvalsWG, 5*time.Second, log)
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
// via slog de forma esparsa (a cada 1%). eng e approvalsWG só são usados para
// o prompt interativo de aprovação (EventJobAwaitingApproval) — ver
// handleAwaitingApproval.
func frameProgress(events chan core.JobEvent, log *slog.Logger, eng *core.Engine, approvalsWG *sync.WaitGroup) {
	render := isTerminal(os.Stderr)
	bar := newProgressBar(os.Stderr)
	var lastPct = map[string]int{}
	for ev := range events {
		switch ev.Kind {
		case core.EventJobStart:
			// Nada a fazer aqui: a barra nasce no primeiro EventJobProgress
			// (chamar bar.endl aqui limparia uma barra que ainda não existe,
			// imprimindo uma linha em branco espúria); o log "job iniciado"
			// já é responsabilidade única de engine.go, evitando duplicação.
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
		case core.EventJobAwaitingApproval:
			if render {
				bar.endl(ev.FilePath)
			}
			// Prompt síncrono roda em goroutine dedicada para não bloquear o
			// consumo dos demais eventos (outros jobs continuam progredindo
			// concorrentemente); approvalsWG permite que main() espere essas
			// goroutines terminarem antes de encerrar o Store (ver waitApprovals).
			approvalsWG.Add(1)
			go func(ev core.JobEvent) {
				defer approvalsWG.Done()
				handleAwaitingApproval(eng, ev, log)
			}(ev)
		}
	}
}

// approvalPromptMu serializa os prompts interativos de aprovação: evita que a
// saída de dois jobs concorrentes se misture no terminal quando mais de um
// job chega a AWAITING_APPROVAL ao mesmo tempo (ex.: -workers > 1).
var approvalPromptMu sync.Mutex

// handleAwaitingApproval reage a EventJobAwaitingApproval. Em terminal TTY
// (stdin e stdout interativos), imprime o resumo do job e lê a decisão y/N
// do usuário, chamando ApproveJob/RejectJob de forma síncrona. Fora de TTY
// (daemon/saída redirecionada), não há para quem perguntar — só loga a
// sugestão dos comandos -approve/-reject para uso posterior via -list-staged.
func handleAwaitingApproval(eng *core.Engine, ev core.JobEvent, log *slog.Logger) {
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		log.Info("job aguardando aprovação manual", "job_id", ev.JobID, "path", ev.FilePath,
			"approve_cmd", "-approve "+ev.JobID, "reject_cmd", "-reject "+ev.JobID)
		return
	}
	approvalPromptMu.Lock()
	defer approvalPromptMu.Unlock()
	printApprovalSummary(ev, os.Stdout)
	if promptApproval(os.Stdin) {
		if err := eng.ApproveJob(ev.JobID); err != nil {
			log.Error("falha ao aprovar job", "job_id", ev.JobID, "error", err.Error())
		}
		return
	}
	if err := eng.RejectJob(ev.JobID); err != nil {
		log.Error("falha ao rejeitar job", "job_id", ev.JobID, "error", err.Error())
	}
}

// printApprovalSummary imprime o resumo do job em staging (tamanhos, economia
// de espaço, codec alvo) que antecede o prompt y/N, no formato descrito em
// docs/propostas_v1.2.md.
func printApprovalSummary(ev core.JobEvent, w io.Writer) {
	m := ev.Metrics
	fmt.Fprintf(w, "\n[%s] Staging Concluído: %q\n", shortID(ev.JobID), filepath.Base(ev.FilePath))
	fmt.Fprintf(w, "  Tamanho Original:    %s\n", humanBytes(m.OriginalSizeBytes))
	fmt.Fprintf(w, "  Tamanho Convertido:  %s\n", humanBytes(m.ConvertedSizeBytes))
	fmt.Fprintf(w, "  Espaço Economizado:  %s (%.2f%%)\n", humanBytes(m.SavedBytes), m.CompressionRatioPct)
	fmt.Fprintf(w, "  Codec Alvo:          %s\n", strings.ToUpper(ev.TargetCodec))
	fmt.Fprint(w, "  Deseja substituir o arquivo original? [y/N]: ")
}

// promptApproval lê uma linha de r e retorna true só para "y"/"yes"
// (case-insensitive) — qualquer outra resposta (incluindo vazia/EOF) é um
// "não" seguro, preservando o arquivo original.
func promptApproval(r io.Reader) bool {
	line, _ := bufio.NewReader(r).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// waitApprovals aguarda os prompts interativos de aprovação em andamento
// (approvalsWG) terminarem antes do chamador prosseguir para eng.Shutdown().
// timeout<=0 espera indefinidamente (modo -file: aguardar a decisão do
// usuário é o comportamento central da Fase 1); timeout>0 desiste após esse
// prazo e loga um aviso (modo daemon/watch — mesmo padrão de Engine.waitWebhooks).
func waitApprovals(wg *sync.WaitGroup, timeout time.Duration, log *slog.Logger) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	if timeout <= 0 {
		<-done
		return
	}
	select {
	case <-done:
	case <-time.After(timeout):
		log.Warn("timeout aguardando aprovações pendentes durante shutdown")
	}
}

// humanBytes formata bytes em unidade adaptativa (B/KB/MB/GB), reaproveitado
// por -list-staged (e, na Fase 6, por -history).
func humanBytes(n int64) string {
	const unit = 1024.0
	f := float64(n)
	switch {
	case f >= unit*unit*unit:
		return fmt.Sprintf("%.2f GB", f/(unit*unit*unit))
	case f >= unit*unit:
		return fmt.Sprintf("%.2f MB", f/(unit*unit))
	case f >= unit:
		return fmt.Sprintf("%.2f KB", f/unit)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// managementArgs agrupa os parâmetros dos comandos administrativos de
// staging/aprovação (-list-staged/-approve/-approve-all/-reject/-reject-all)
// e do histórico filtrável de jobs (-history, Fase 6).
type managementArgs struct {
	storePath  string
	rulesPath  string
	listStaged bool
	approveID  string
	approveAll bool
	rejectID   string
	rejectAll  bool

	history       bool
	historyStatus string
	historySince  string
}

// runManagementCommand executa um subcomando administrativo e retorna o exit
// code do processo. Abre Store+RulesEngine via buildEngine (events=nil,
// Logger=nil → default interno de NewEngine) e NUNCA chama
// WatchDir/StartWatch/OpenWorkers — nenhuma goroutine de worker/watcher sobe,
// já que o objetivo é só consultar/alterar o Store existente e sair.
func runManagementCommand(a managementArgs, out io.Writer) int {
	eng, err := buildEngine(a.storePath, a.rulesPath, 1, nil, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		return 1
	}
	defer eng.Shutdown()

	switch {
	case a.listStaged:
		return cmdListStaged(eng, out)
	case a.approveID != "":
		return cmdApprove(eng, a.approveID, out)
	case a.approveAll:
		return cmdApproveAll(eng, out)
	case a.rejectID != "":
		return cmdReject(eng, a.rejectID, out)
	case a.rejectAll:
		return cmdRejectAll(eng, out)
	case a.history:
		return cmdHistory(eng, a.historyStatus, a.historySince, out)
	}
	return 0
}

func cmdListStaged(eng *core.Engine, out io.Writer) int {
	jobs, err := eng.ListStaged()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro ao listar jobs pendentes:", err)
		return 1
	}
	if len(jobs) == 0 {
		fmt.Fprintln(out, "nenhum job aguardando aprovação")
		return 0
	}
	formatStagedTable(jobs, out)
	return 0
}

func cmdApprove(eng *core.Engine, id string, out io.Writer) int {
	if err := eng.ApproveJob(id); err != nil {
		fmt.Fprintln(os.Stderr, "erro ao aprovar job:", err)
		return 1
	}
	fmt.Fprintf(out, "job %s aprovado e finalizado\n", shortID(id))
	return 0
}

func cmdReject(eng *core.Engine, id string, out io.Writer) int {
	if err := eng.RejectJob(id); err != nil {
		fmt.Fprintln(os.Stderr, "erro ao rejeitar job:", err)
		return 1
	}
	fmt.Fprintf(out, "job %s rejeitado (original preservado)\n", shortID(id))
	return 0
}

func cmdApproveAll(eng *core.Engine, out io.Writer) int {
	n, errs := eng.ApproveAll()
	fmt.Fprintf(out, "%d job(s) aprovado(s)\n", n)
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "erro:", e)
	}
	if len(errs) > 0 {
		return 1
	}
	return 0
}

func cmdRejectAll(eng *core.Engine, out io.Writer) int {
	n, errs := eng.RejectAll()
	fmt.Fprintf(out, "%d job(s) rejeitado(s)\n", n)
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "erro:", e)
	}
	if len(errs) > 0 {
		return 1
	}
	return 0
}

// cmdHistory lista o histórico de jobs filtrado por status/since (Fase 6).
// status é comparado livremente contra core.JobStatus (sem validação contra
// uma lista fechada de valores — um status inexistente simplesmente não bate
// com nenhum job e retorna lista vazia, mesma tolerância de FindByPath/etc.
// para strings vindas do usuário). since usa o mesmo padrão de -scan-interval
// (time.ParseDuration), convertido para Since=time.Now().Add(-duração).
func cmdHistory(eng *core.Engine, status, since string, out io.Writer) int {
	filter := core.JobFilter{}
	if status != "" {
		filter.Status = core.JobStatus(status)
	}
	if since != "" {
		d, err := time.ParseDuration(since)
		if err != nil {
			fmt.Fprintln(os.Stderr, "erro: -since inválido:", err)
			return 1
		}
		t := time.Now().Add(-d)
		filter.Since = &t
	}
	jobs, err := eng.ListJobs(filter)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro ao listar histórico:", err)
		return 1
	}
	if len(jobs) == 0 {
		fmt.Fprintln(out, "nenhum job encontrado para o filtro informado")
		return 0
	}
	formatHistoryTable(jobs, out)
	return 0
}

// formatHistoryTable imprime a tabela do histórico de jobs (-history, Fase
// 6): ID/Arquivo/Status/Original/Convertido/Economia/Finalizado em — mesmo
// estilo visual de formatStagedTable, com a coluna extra de Status (o
// histórico mistura jobs em vários estados finais) e Finalizado em (quando
// FinishedAt existe; jobs ainda em andamento mostram "-").
func formatHistoryTable(jobs []*core.Job, w io.Writer) {
	fmt.Fprintf(w, "%-10s %-30s %-12s %-10s %-12s %-10s %s\n",
		"ID", "Arquivo", "Status", "Original", "Convertido", "Economia", "Finalizado em")
	for _, j := range jobs {
		m := j.SizeMetrics
		economia := fmt.Sprintf("%.2f%%", m.CompressionRatioPct)
		finalizado := "-"
		if j.FinishedAt != nil {
			finalizado = j.FinishedAt.Local().Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(w, "%-10s %-30s %-12s %-10s %-12s %-10s %s\n",
			shortID(j.ID), filepath.Base(j.Path), string(j.Status),
			humanBytes(m.OriginalSizeBytes), humanBytes(m.ConvertedSizeBytes), economia, finalizado)
	}
}

// formatStagedTable imprime a tabela de jobs aguardando aprovação no formato
// descrito em docs/propostas_v1.2.md (ID/Arquivo/Original/Convertido/Economia
// + sugestão do comando -approve pronto para copiar/colar).
func formatStagedTable(jobs []*core.Job, w io.Writer) {
	fmt.Fprintf(w, "%-10s %-30s %-10s %-12s %s\n", "ID", "Arquivo", "Original", "Convertido", "Economia")
	for _, j := range jobs {
		m := j.SizeMetrics
		economia := fmt.Sprintf("%.2f%% (Aprovar: ./codecany -approve %s)", m.CompressionRatioPct, j.ID)
		fmt.Fprintf(w, "%-10s %-30s %-10s %-12s %s\n",
			shortID(j.ID), filepath.Base(j.Path), humanBytes(m.OriginalSizeBytes), humanBytes(m.ConvertedSizeBytes), economia)
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
