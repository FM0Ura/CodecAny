package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FM0Ura/codecany/pkg/core"
)

// fakeVerifier implementa core.MediaVerifier para os testes de
// runHealthCheck (Fase 5). main_test.go é package main e não pode importar
// tipos não-exportados de pkg/core (ex.: mockVerifier de mocks_test.go), daí
// este fake local — err é indexado por path, permitindo configurar respostas
// diferentes por arquivo dentro de um mesmo caso de teste.
type fakeVerifier struct {
	errByPath map[string]error
}

func (f fakeVerifier) Verify(path string) error {
	return f.errByPath[path]
}

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
		Metrics: &core.SizeMetrics{
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

// TestRunHealthCheckDetectsCorrupted cobre o caso central do modo
// -health-check (Fase 5): um arquivo corrompido entre vários deve gerar a
// linha [CORRUPTED] correta (com o erro do verifier) e fazer a função
// retornar exit code 1 — mesmo com outros arquivos OK.
func TestRunHealthCheckDetectsCorrupted(t *testing.T) {
	ok := "/media/ok.mkv"
	bad := "/media/bad.mkv"
	verifier := fakeVerifier{errByPath: map[string]error{
		bad: errors.New("erro de decodificação"),
	}}

	var buf bytes.Buffer
	code := runHealthCheck(nil, []string{ok, bad}, verifier, &buf, false)

	if code != 1 {
		t.Fatalf("exit code = %d, esperava 1", code)
	}
	out := buf.String()
	if !strings.Contains(out, "[OK] "+ok) {
		t.Errorf("esperava linha [OK] para %s; obteve:\n%s", ok, out)
	}
	if !strings.Contains(out, "[CORRUPTED] "+bad+": erro de decodificação") {
		t.Errorf("esperava linha [CORRUPTED] com o erro para %s; obteve:\n%s", bad, out)
	}
}

// TestRunHealthCheckAllOK garante exit code 0 e uma linha [OK] por arquivo
// quando nenhum arquivo está corrompido.
func TestRunHealthCheckAllOK(t *testing.T) {
	a := "/media/a.mkv"
	b := "/media/b.mp4"
	verifier := fakeVerifier{}

	var buf bytes.Buffer
	code := runHealthCheck(nil, []string{a, b}, verifier, &buf, false)

	if code != 0 {
		t.Fatalf("exit code = %d, esperava 0", code)
	}
	out := buf.String()
	for _, want := range []string{"[OK] " + a, "[OK] " + b} {
		if !strings.Contains(out, want) {
			t.Errorf("esperava %q na saída; obteve:\n%s", want, out)
		}
	}
	if strings.Contains(out, "CORRUPTED") {
		t.Errorf("não deveria haver nenhuma linha CORRUPTED; obteve:\n%s", out)
	}
}

// TestRunHealthCheckJSONOutput confirma o formato JSON por linha (uma por
// arquivo) quando jsonOut=true.
func TestRunHealthCheckJSONOutput(t *testing.T) {
	ok := "/media/ok.mkv"
	bad := "/media/bad.mkv"
	verifier := fakeVerifier{errByPath: map[string]error{
		bad: errors.New("boom"),
	}}

	var buf bytes.Buffer
	code := runHealthCheck(nil, []string{ok, bad}, verifier, &buf, true)

	if code != 1 {
		t.Fatalf("exit code = %d, esperava 1", code)
	}
	out := buf.String()
	if !strings.Contains(out, `"path":"`+ok+`"`) || !strings.Contains(out, `"status":"ok"`) {
		t.Errorf("esperava linha JSON de status ok para %s; obteve:\n%s", ok, out)
	}
	if !strings.Contains(out, `"path":"`+bad+`"`) || !strings.Contains(out, `"status":"corrupted"`) || !strings.Contains(out, `"error":"boom"`) {
		t.Errorf("esperava linha JSON de status corrupted com erro para %s; obteve:\n%s", bad, out)
	}
}

// TestRunHealthCheckDiscoversDirs confirma que dirs é varrido via
// core.DiscoverFiles (mesmo filtro de extensão do watcher), incluindo apenas
// arquivos de mídia suportados e ignorando os demais — sem enfileirar nada,
// só chamando o verifier injetado.
func TestRunHealthCheckDiscoversDirs(t *testing.T) {
	dir := t.TempDir()
	media := dir + "/movie.mkv"
	if err := os.WriteFile(media, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	notMedia := dir + "/readme.txt"
	if err := os.WriteFile(notMedia, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	verifier := fakeVerifier{}
	var buf bytes.Buffer
	code := runHealthCheck([]string{dir}, nil, verifier, &buf, false)

	if code != 0 {
		t.Fatalf("exit code = %d, esperava 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, "[OK] "+media) {
		t.Errorf("esperava %s descoberto e verificado; obteve:\n%s", media, out)
	}
	if strings.Contains(out, notMedia) {
		t.Errorf("readme.txt não deveria ter sido descoberto; obteve:\n%s", out)
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
