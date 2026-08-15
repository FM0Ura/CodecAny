package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FM0Ura/codecany/pkg/core"
)

func TestProgressBarUpdate(t *testing.T) {
	var buf bytes.Buffer
	b := newProgressBar(&buf)
	b.update("12345678", "/tmp/data/arquivoo.mkv", 42.0)
	line := buf.String()

	if !strings.HasPrefix(line, "\r[12345678]") {
		t.Errorf("barra deveria começar com \\r + id; obteve: %q", line)
	}
	if !strings.Contains(line, "arquivoo.mkv") {
		t.Errorf("barra deveria conter o nome do arquivo; obteve: %q", line)
	}
	if !strings.Contains(line, " 42.0000%") {
		t.Errorf("barra deveria conter o percentual 42; obteve: %q", line)
	}
}

func TestProgressBarClamps(t *testing.T) {
	var buf bytes.Buffer
	b := newProgressBar(&buf)
	b.update("12345678", "a.mkv", 150.0) // >100 deve virar 100
	out := buf.String()
	if !strings.Contains(out, "100.0000%") {
		t.Errorf("percentual deveria ser limitado a 100; obteve: %q", out)
	}
	buf.Reset()
	b.update("12345678", "a.mkv", -10.0) // <0 deve virar 0
	if !strings.Contains(buf.String(), "  0.0000%") {
		t.Errorf("percentual deveria ser limitado a 0; obteve: %q", buf.String())
	}
}

func TestHumanBytesFormatsMBAndGB(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{1536, "1.50 KB"},                // 1.5 KB
		{640 * 1024 * 1024, "640.00 MB"}, // 640 MB
		{1524713390, "1.42 GB"},
	}
	for _, c := range cases {
		got := humanBytes(c.bytes)
		if got != c.want {
			t.Errorf("humanBytes(%d) = %q, esperado %q", c.bytes, got, c.want)
		}
	}
}

func TestPrintApprovalSummaryFormatsFields(t *testing.T) {
	var buf bytes.Buffer
	ev := core.JobEvent{
		Kind:        core.EventJobAwaitingApproval,
		JobID:       "1a418284-aaaa-bbbb-cccc-ddddeeeeffff",
		FilePath:    "/videos/The Testament of Sister New Devil.mkv",
		TargetCodec: "av1",
		Metrics: core.SizeMetrics{
			OriginalSizeBytes:   1524000000,
			ConvertedSizeBytes:  671350000,
			SavedBytes:          852650000,
			CompressionRatioPct: 55.94,
		},
	}
	printApprovalSummary(ev, &buf)
	out := buf.String()

	for _, want := range []string{
		"[1a418284]",
		"The Testament of Sister New Devil.mkv",
		"AV1",
		"55.94%",
		"[y/N]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("resumo deveria conter %q; obteve: %s", want, out)
		}
	}
}

func TestFormatStagedTableColumns(t *testing.T) {
	var buf bytes.Buffer
	jobs := []*core.Job{
		{
			ID:   "1a418284-aaaa-bbbb-cccc-ddddeeeeffff",
			Path: "/videos/movie1.mkv",
			SizeMetrics: core.SizeMetrics{
				OriginalSizeBytes:   1524000000,
				ConvertedSizeBytes:  671000000,
				CompressionRatioPct: 56.18,
			},
		},
		{
			ID:   "fe3311ab-aaaa-bbbb-cccc-ddddeeeeffff",
			Path: "/videos/movie2.mkv",
			SizeMetrics: core.SizeMetrics{
				OriginalSizeBytes:   980000000,
				ConvertedSizeBytes:  810000000,
				CompressionRatioPct: 17.34,
			},
		},
	}
	formatStagedTable(jobs, &buf)
	out := buf.String()

	for _, want := range []string{
		"ID", "Arquivo", "Original", "Convertido", "Economia",
		"1a418284", "movie1.mkv", "56.18%",
		"fe3311ab", "movie2.mkv", "17.34%",
		"-approve " + jobs[0].ID,
		"-approve " + jobs[1].ID,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("tabela deveria conter %q; obteve:\n%s", want, out)
		}
	}
}

// TestFormatHistoryTableColumns exercita a tabela do -history (Fase 6):
// precisa trazer a coluna de Status (o histórico mistura jobs em vários
// estados finais, diferente de -list-staged que só lista AWAITING_APPROVAL)
// e o timestamp de Finalizado em quando FinishedAt está preenchido — jobs
// sem FinishedAt (ainda em andamento) devem mostrar "-" em vez de zerar a data.
func TestFormatHistoryTableColumns(t *testing.T) {
	var buf bytes.Buffer
	finishedAt := time.Date(2026, 8, 10, 14, 30, 0, 0, time.UTC)
	jobs := []*core.Job{
		{
			ID:         "1a418284-aaaa-bbbb-cccc-ddddeeeeffff",
			Path:       "/videos/movie1.mkv",
			Status:     core.StatusCompleted,
			FinishedAt: &finishedAt,
			SizeMetrics: core.SizeMetrics{
				OriginalSizeBytes:   1524000000,
				ConvertedSizeBytes:  671000000,
				CompressionRatioPct: 56.18,
			},
		},
		{
			ID:     "fe3311ab-aaaa-bbbb-cccc-ddddeeeeffff",
			Path:   "/videos/movie2.mkv",
			Status: core.StatusFailed,
			// FinishedAt nil: job ainda sem timestamp de término registrado.
		},
	}
	formatHistoryTable(jobs, &buf)
	out := buf.String()

	for _, want := range []string{
		"ID", "Arquivo", "Status", "Original", "Convertido", "Economia", "Finalizado em",
		"1a418284", "movie1.mkv", "COMPLETED", "56.18%",
		"fe3311ab", "movie2.mkv", "FAILED",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("tabela deveria conter %q; obteve:\n%s", want, out)
		}
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("esperava 3 linhas (cabeçalho + 2 jobs), obteve %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[2], " - ") && !strings.HasSuffix(lines[2], "-") {
		t.Errorf("job sem FinishedAt deveria mostrar \"-\" na coluna final; linha: %q", lines[2])
	}
}

// TestCmdHistoryFormatsFilterSelection exercita o parse de -status/-since via
// cmdHistory ponta a ponta contra um Store real: cria jobs com CreatedAt
// variados e confirma que o filtro textual vindo da CLI (string -status,
// duração -since) chega corretamente em core.JobFilter e retorna as linhas
// certas na tabela.
func TestCmdHistoryFormatsFilterSelection(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	s, err := core.NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	old := &core.Job{ID: "job-old", Path: "old.mkv", Status: core.StatusCompleted, Driver: "ffmpeg",
		CreatedAt: now.Add(-48 * time.Hour)}
	recent := &core.Job{ID: "job-recent", Path: "recent.mkv", Status: core.StatusFailed, Driver: "ffmpeg",
		CreatedAt: now.Add(-1 * time.Hour)}
	for _, j := range []*core.Job{old, recent} {
		if err := s.CreateJob(j); err != nil {
			t.Fatal(err)
		}
	}

	eng, err := core.NewEngine(core.EngineDeps{
		Store: s,
		Rules: mustTestRulesEngine(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Shutdown()

	var buf bytes.Buffer
	code := cmdHistory(eng, "FAILED", "24h", &buf)
	if code != 0 {
		t.Fatalf("cmdHistory retornou código %d, esperava 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, "recent.mkv") {
		t.Errorf("esperava recent.mkv (FAILED, dentro de 24h) na saída: %s", out)
	}
	if strings.Contains(out, "old.mkv") {
		t.Errorf("old.mkv não deveria aparecer (fora da janela -since=24h): %s", out)
	}
}

// mustTestRulesEngine constrói um core.RulesEngine mínimo (só o necessário
// para NewEngine montar staging_dir/space_saving/notifications) a partir de
// um rules.yaml temporário — os testes de -history não exercitam nenhuma
// regra de conversão, só precisam de um Engine válido para chamar ListJobs.
func mustTestRulesEngine(t *testing.T) *core.RulesEngine {
	t.Helper()
	content := `
version: 1
global:
  staging_dir: ` + t.TempDir() + `
  space_saving:
    min_saving_pct: 15
rules:
  - name: "h264 -> hevc"
    match:
      container: mkv
      video: { codec: h264 }
    convert:
      video: { codec: hevc, crf: 22, preset: medium }
`
	path := t.TempDir() + "/rules.yaml"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	re, err := core.NewRulesEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	return re
}
