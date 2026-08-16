package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FM0Ura/codecany/pkg/core"
)

// fakeProber/fakeVerifier/fakeTranscoder implementam
// core.MediaProber/core.MediaVerifier/core.TranscoderEngine — package main
// não pode importar tipos/mocks não-exportados de pkg/core (mesmo motivo de
// fakeVerifier em cmd/cli/main_test.go), e a Fase A do servidor não precisa
// de um ffmpeg de verdade: os testes seguem só a superfície de leitura da
// API, sem disparar o pipeline de transcode.
type fakeProber struct{}

func (fakeProber) Probe(path string) (core.MediaInfo, error) {
	return core.MediaInfo{Path: path}, nil
}

type fakeVerifier struct{}

func (fakeVerifier) Verify(path string) error { return nil }

type fakeTranscoder struct{}

func (fakeTranscoder) Name() string { return "ffmpeg" }
func (fakeTranscoder) Transcode(input, output string, media core.MediaInfo, target core.TargetSpec) (<-chan float64, error) {
	ch := make(chan float64)
	close(ch)
	return ch, nil
}

// testServer agrupa as dependências de um App montado sobre um Store real
// em tempdir + Engine real com adapters fake, prontos para
// httptest.NewServer. store é devolvido separadamente para permitir semear
// jobs diretamente (bypassando o pipeline de probe/regras/transcode, que
// esta fase não exercita).
type testServer struct {
	app    *App
	store  *core.Store
	events chan core.JobEvent
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "x.db")
	rulesPath := filepath.Join(dir, "rules.yaml")

	writeFixtureRules(t, rulesPath)

	store, err := core.NewStore(dbPath)
	if err != nil {
		t.Fatalf("core.NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	rules, err := core.NewRulesEngine(rulesPath)
	if err != nil {
		t.Fatalf("core.NewRulesEngine: %v", err)
	}

	events := make(chan core.JobEvent, 16)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	eng, err := core.NewEngine(core.EngineDeps{
		Store:    store,
		Rules:    rules,
		Prober:   fakeProber{},
		Verifier: fakeVerifier{},
		Engines:  []core.TranscoderEngine{fakeTranscoder{}},
		Workers:  1,
		Events:   events,
		Logger:   log,
	})
	if err != nil {
		t.Fatalf("core.NewEngine: %v", err)
	}

	broadcaster := NewBroadcaster()
	go broadcaster.Run(eng.Events())

	assets, err := newAssetHandler()
	if err != nil {
		t.Fatalf("newAssetHandler: %v", err)
	}

	app := &App{
		Engine:      eng,
		Rules:       rules,
		Prober:      fakeProber{},
		Broadcaster: broadcaster,
		Config: ServerConfig{
			Addr:      "127.0.0.1:8383",
			DBPath:    dbPath,
			RulesPath: rulesPath,
			Workers:   1,
		},
		StartedAt: time.Now(),
		Assets:    assets,
		Log:       log,
	}

	return &testServer{app: app, store: store, events: events}
}

// writeFixtureRules grava um rules.yaml mínimo válido (NewRulesEngine exige
// pelo menos uma regra).
func writeFixtureRules(t *testing.T, path string) {
	t.Helper()
	const fixture = `
version: 1
global:
  default_driver: ffmpeg
  staging_dir: .codecany_tmp
  space_saving:
    min_saving_pct: 15
rules:
  - name: "h264 para av1"
    match:
      video:
        codec: h264
    action: convert
    convert:
      video:
        codec: av1
`
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatalf("escrever fixture de regras: %v", err)
	}
}

// seedJob persiste um Job diretamente via Store (sem passar pelo pipeline de
// probe/regras/transcode do Engine) — suficiente para os endpoints
// somente-leitura desta fase.
func (ts *testServer) seedJob(t *testing.T, status core.JobStatus, m core.SizeMetrics) *core.Job {
	t.Helper()
	job := &core.Job{
		ID:        fmt.Sprintf("job-%d-%s", time.Now().UnixNano(), status),
		Path:      "/media/" + string(status) + ".mkv",
		Status:    status,
		Driver:    "ffmpeg",
		CreatedAt: time.Now(),
	}
	if err := ts.store.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := ts.store.UpdateMetrics(job.ID, m); err != nil {
		t.Fatalf("UpdateMetrics: %v", err)
	}
	job.SizeMetrics = m
	return job
}

func TestHandleStatus(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/status")
	if err != nil {
		t.Fatalf("GET /api/status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Addr != ts.app.Config.Addr {
		t.Errorf("addr = %q, want %q", got.Addr, ts.app.Config.Addr)
	}
	if got.DBPath != ts.app.Config.DBPath {
		t.Errorf("db_path = %q, want %q", got.DBPath, ts.app.Config.DBPath)
	}
	if got.RulesPath != ts.app.Config.RulesPath {
		t.Errorf("rules_path = %q, want %q", got.RulesPath, ts.app.Config.RulesPath)
	}
	if got.Workers != ts.app.Config.Workers {
		t.Errorf("workers = %d, want %d", got.Workers, ts.app.Config.Workers)
	}
	if got.Version == "" {
		t.Error("version vazio")
	}
	if got.UptimeS < 0 {
		t.Errorf("uptime_s = %f, want >= 0", got.UptimeS)
	}
}

func TestHandleDashboardSummary(t *testing.T) {
	ts := newTestServer(t)

	ts.seedJob(t, core.StatusCompleted, core.SizeMetrics{OriginalSizeBytes: 1000, ConvertedSizeBytes: 400, SavedBytes: 600})
	ts.seedJob(t, core.StatusCompleted, core.SizeMetrics{OriginalSizeBytes: 2000, ConvertedSizeBytes: 800, SavedBytes: 1200})
	ts.seedJob(t, core.StatusFailed, core.SizeMetrics{})
	ts.seedJob(t, core.StatusQueued, core.SizeMetrics{})
	ts.seedJob(t, core.StatusQueued, core.SizeMetrics{})

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/dashboard/summary")
	if err != nil {
		t.Fatalf("GET /api/dashboard/summary: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got dashboardSummary
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.StatusCounts[string(core.StatusCompleted)] != 2 {
		t.Errorf("status_counts[COMPLETED] = %d, want 2", got.StatusCounts[string(core.StatusCompleted)])
	}
	if got.StatusCounts[string(core.StatusFailed)] != 1 {
		t.Errorf("status_counts[FAILED] = %d, want 1", got.StatusCounts[string(core.StatusFailed)])
	}
	if got.StatusCounts[string(core.StatusQueued)] != 2 {
		t.Errorf("status_counts[QUEUED] = %d, want 2", got.StatusCounts[string(core.StatusQueued)])
	}
	if got.TotalSavings.OriginalSizeBytes != 3000 {
		t.Errorf("total_savings.original_size_bytes = %d, want 3000", got.TotalSavings.OriginalSizeBytes)
	}
	if got.TotalSavings.SavedBytes != 1800 {
		t.Errorf("total_savings.saved_bytes = %d, want 1800", got.TotalSavings.SavedBytes)
	}
	if got.HWAccel == nil {
		t.Error("hwaccel não deveria ser nil (deve ser [] quando vazio)")
	}
}

func TestHandleListJobsAndGetJob(t *testing.T) {
	ts := newTestServer(t)
	completed := ts.seedJob(t, core.StatusCompleted, core.SizeMetrics{OriginalSizeBytes: 500, SavedBytes: 100})
	ts.seedJob(t, core.StatusFailed, core.SizeMetrics{})

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	// GET /api/jobs sem filtro: deve trazer os dois.
	resp, err := http.Get(srv.URL + "/api/jobs")
	if err != nil {
		t.Fatalf("GET /api/jobs: %v", err)
	}
	var all []core.Job
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if len(all) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(all))
	}

	// GET /api/jobs?status=COMPLETED: só o completed.
	resp, err = http.Get(srv.URL + "/api/jobs?status=" + string(core.StatusCompleted))
	if err != nil {
		t.Fatalf("GET /api/jobs?status=: %v", err)
	}
	var filtered []core.Job
	if err := json.NewDecoder(resp.Body).Decode(&filtered); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if len(filtered) != 1 || filtered[0].ID != completed.ID {
		t.Fatalf("filtro status=COMPLETED trouxe %d jobs, want 1 (%v)", len(filtered), filtered)
	}

	// GET /api/jobs/{id} existente.
	resp, err = http.Get(srv.URL + "/api/jobs/" + completed.ID)
	if err != nil {
		t.Fatalf("GET /api/jobs/{id}: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got core.Job
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if got.ID != completed.ID {
		t.Errorf("id = %q, want %q", got.ID, completed.ID)
	}

	// GET /api/jobs/{id} inexistente → 404.
	resp, err = http.Get(srv.URL + "/api/jobs/nao-existe")
	if err != nil {
		t.Fatalf("GET /api/jobs/nao-existe: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestHandleJobsSinceInvalid(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/jobs?since=not-a-duration")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSSEEvents(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %q, want text/event-stream", ct)
	}

	// Dá tempo do handler registrar o canal-cliente antes de publicar —
	// evita a corrida de publicar antes do register() rodar.
	time.Sleep(50 * time.Millisecond)

	ts.events <- core.JobEvent{Kind: core.EventJobStart, JobID: "abc123", FilePath: "/media/foo.mkv", Driver: "ffmpeg"}

	reader := bufio.NewReader(resp.Body)
	var eventLine, dataLine string
	for i := 0; i < 2; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("ler frame SSE: %v", err)
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			eventLine = line
		case strings.HasPrefix(line, "data:"):
			dataLine = line
		}
	}

	if !strings.Contains(eventLine, string(core.EventJobStart)) {
		t.Errorf("event line = %q, want conter %q", eventLine, core.EventJobStart)
	}
	var ev core.JobEvent
	jsonPart := strings.TrimSpace(strings.TrimPrefix(dataLine, "data:"))
	if err := json.Unmarshal([]byte(jsonPart), &ev); err != nil {
		t.Fatalf("decode data JSON (%q): %v", jsonPart, err)
	}
	if ev.JobID != "abc123" {
		t.Errorf("job_id = %q, want abc123", ev.JobID)
	}
}

func TestSPAFallback(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	indexResp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	indexBody, err := io.ReadAll(indexResp.Body)
	indexResp.Body.Close()
	if err != nil {
		t.Fatalf("ler body de /: %v", err)
	}

	unknownResp, err := http.Get(srv.URL + "/dashboard/qualquer-coisa")
	if err != nil {
		t.Fatalf("GET /dashboard/qualquer-coisa: %v", err)
	}
	defer unknownResp.Body.Close()
	if unknownResp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (fallback SPA)", unknownResp.StatusCode)
	}
	unknownBody, err := io.ReadAll(unknownResp.Body)
	if err != nil {
		t.Fatalf("ler body do fallback: %v", err)
	}
	if string(unknownBody) != string(indexBody) {
		t.Errorf("fallback não retornou o mesmo conteúdo de index.html:\n got=%q\nwant=%q", unknownBody, indexBody)
	}
	if !strings.Contains(string(unknownBody), "<html") {
		t.Errorf("fallback não parece HTML: %q", unknownBody)
	}
}
