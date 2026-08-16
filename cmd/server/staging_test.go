package main

import (
	"bytes"
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

// newTestServerWithStagingDir espelha newTestServer (router_test.go), mas com
// um staging_dir ABSOLUTO em tempdir (em vez de ".codecany_tmp" relativo ao
// cwd do processo de teste) — necessário aqui porque, diferente dos testes de
// router_test.go, os testes deste arquivo exercitam de verdade
// ApproveJob/RejectJob (core.Cleanup faz rename/replace de arquivos reais em
// disco a partir de staging_dir), então o caminho precisa ser hermético ao
// tempdir do teste.
func newTestServerWithStagingDir(t *testing.T, stagingDir string) *testServer {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "x.db")
	rulesPath := filepath.Join(dir, "rules.yaml")

	fixture := fmt.Sprintf(`
version: 1
global:
  default_driver: ffmpeg
  staging_dir: %s
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
`, stagingDir)
	if err := os.WriteFile(rulesPath, []byte(fixture), 0o644); err != nil {
		t.Fatalf("escrever fixture de regras: %v", err)
	}

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

// seedStagedJob cria um Job real em StatusAwaitingApproval: um arquivo
// original em disco (mediaDir, fora do staging) e o output correspondente já
// presente em staging_dir/<base>/output<ext> — exatamente o que
// core.NewCleanup espera encontrar para Commit/Abort funcionarem de verdade
// (mesma mecânica de pkg/core/engine_test.go::setupEngineAwaitingApproval,
// adaptada para o Job já nascer em AWAITING_APPROVAL sem passar pelo
// pipeline completo do Engine).
func (ts *testServer) seedStagedJob(t *testing.T, stagingDir, name string, origSize, convSize int64) *core.Job {
	t.Helper()
	mediaDir := t.TempDir()
	mediaPath := filepath.Join(mediaDir, name+".mkv")
	if err := os.WriteFile(mediaPath, bytes.Repeat([]byte{0xAB}, int(origSize)), 0o644); err != nil {
		t.Fatalf("escrever mídia original: %v", err)
	}

	stageSub := filepath.Join(stagingDir, name)
	if err := os.MkdirAll(stageSub, 0o755); err != nil {
		t.Fatalf("criar staging: %v", err)
	}
	outputPath := filepath.Join(stageSub, "output.mkv")
	if err := os.WriteFile(outputPath, bytes.Repeat([]byte{0xCD}, int(convSize)), 0o644); err != nil {
		t.Fatalf("escrever output em staging: %v", err)
	}

	job := &core.Job{
		ID:        fmt.Sprintf("job-%s-%d", name, time.Now().UnixNano()),
		Path:      mediaPath,
		Status:    core.StatusAwaitingApproval,
		Driver:    "ffmpeg",
		Target:    core.TargetSpec{VideoCodec: "av1", Container: "mkv"},
		CreatedAt: time.Now(),
	}
	if err := ts.store.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	m := core.SizeMetrics{
		OriginalSizeBytes:  origSize,
		ConvertedSizeBytes: convSize,
		SavedBytes:         origSize - convSize,
	}
	if err := ts.store.UpdateMetrics(job.ID, m); err != nil {
		t.Fatalf("UpdateMetrics: %v", err)
	}
	job.SizeMetrics = m
	return job
}

func postJSON(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func TestHandleListStaging(t *testing.T) {
	stagingDir := t.TempDir()
	ts := newTestServerWithStagingDir(t, stagingDir)
	ts.seedStagedJob(t, stagingDir, "movie-a", 1000, 400)
	ts.seedStagedJob(t, stagingDir, "movie-b", 2000, 900)
	ts.seedJob(t, core.StatusCompleted, core.SizeMetrics{OriginalSizeBytes: 500, SavedBytes: 100})

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/staging")
	if err != nil {
		t.Fatalf("GET /api/staging: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got []core.Job
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(staging) = %d, want 2 (só AWAITING_APPROVAL)", len(got))
	}
	for _, j := range got {
		if j.Status != core.StatusAwaitingApproval {
			t.Errorf("job %s com status %v, want AWAITING_APPROVAL", j.ID, j.Status)
		}
	}
}

func TestHandleListStagingEmpty(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/staging")
	if err != nil {
		t.Fatalf("GET /api/staging: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("body = %q, want [] (nunca null)", body)
	}
}

func TestHandleApproveJobSuccess(t *testing.T) {
	stagingDir := t.TempDir()
	ts := newTestServerWithStagingDir(t, stagingDir)
	job := ts.seedStagedJob(t, stagingDir, "movie", 1000, 400)

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/staging/"+job.ID+"/approve")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}
	var got okResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.OK {
		t.Error("ok = false, want true")
	}

	// Original trocado pelo conteúdo convertido (400 bytes).
	fi, err := os.Stat(job.Path)
	if err != nil {
		t.Fatalf("stat original: %v", err)
	}
	if fi.Size() != 400 {
		t.Errorf("tamanho final = %d, want 400 (arquivo trocado)", fi.Size())
	}

	// Staging purgada.
	if _, err := os.Stat(filepath.Join(stagingDir, "movie")); !os.IsNotExist(err) {
		t.Errorf("staging deveria ter sido purgada, err=%v", err)
	}

	persisted, err := ts.app.Engine.GetJob(job.ID)
	if err != nil || persisted == nil {
		t.Fatalf("GetJob após approve: %v", err)
	}
	if persisted.Status != core.StatusCompleted {
		t.Errorf("status persistido = %v, want COMPLETED", persisted.Status)
	}
}

func TestHandleRejectJobSuccess(t *testing.T) {
	stagingDir := t.TempDir()
	ts := newTestServerWithStagingDir(t, stagingDir)
	job := ts.seedStagedJob(t, stagingDir, "movie", 1000, 400)

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/staging/"+job.ID+"/reject")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}
	var got okResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.OK {
		t.Error("ok = false, want true")
	}

	// Original preservado intocado (1000 bytes).
	fi, err := os.Stat(job.Path)
	if err != nil {
		t.Fatalf("stat original: %v", err)
	}
	if fi.Size() != 1000 {
		t.Errorf("tamanho do original = %d, want 1000 (preservado)", fi.Size())
	}

	persisted, err := ts.app.Engine.GetJob(job.ID)
	if err != nil || persisted == nil {
		t.Fatalf("GetJob após reject: %v", err)
	}
	if persisted.Status != core.StatusRolledBack {
		t.Errorf("status persistido = %v, want ROLLED_BACK", persisted.Status)
	}
}

func TestHandleApproveJobNotFound(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/staging/nao-existe/approve")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestHandleRejectJobNotFound(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/staging/nao-existe/reject")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestHandleApproveJobWrongStatus confirma que um job existente mas que já
// não está mais aguardando aprovação (ex.: já COMPLETED) também vira 404, não
// 500 — mesmo tratamento de "não está em staging" da proposta.
func TestHandleApproveJobWrongStatus(t *testing.T) {
	ts := newTestServer(t)
	job := ts.seedJob(t, core.StatusCompleted, core.SizeMetrics{})

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/staging/"+job.ID+"/approve")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestHandleApproveAll(t *testing.T) {
	stagingDir := t.TempDir()
	ts := newTestServerWithStagingDir(t, stagingDir)
	ts.seedStagedJob(t, stagingDir, "movie-a", 1000, 400)
	ts.seedStagedJob(t, stagingDir, "movie-b", 2000, 900)
	ts.seedStagedJob(t, stagingDir, "movie-c", 3000, 1200)

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/staging/approve-all")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}
	var got approveAllResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Approved != 3 {
		t.Errorf("approved = %d, want 3", got.Approved)
	}
	if len(got.Errors) != 0 {
		t.Errorf("errors = %v, want vazio", got.Errors)
	}

	staged, err := ts.app.Engine.ListStaged()
	if err != nil {
		t.Fatalf("ListStaged: %v", err)
	}
	if len(staged) != 0 {
		t.Errorf("len(staged) = %d, want 0 após approve-all", len(staged))
	}
}

func TestHandleRejectAll(t *testing.T) {
	stagingDir := t.TempDir()
	ts := newTestServerWithStagingDir(t, stagingDir)
	j1 := ts.seedStagedJob(t, stagingDir, "movie-a", 1000, 400)
	j2 := ts.seedStagedJob(t, stagingDir, "movie-b", 2000, 900)

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/staging/reject-all")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200 (body=%s)", resp.StatusCode, body)
	}
	var got rejectAllResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Rejected != 2 {
		t.Errorf("rejected = %d, want 2", got.Rejected)
	}
	if len(got.Errors) != 0 {
		t.Errorf("errors = %v, want vazio", got.Errors)
	}

	for _, j := range []*core.Job{j1, j2} {
		fi, err := os.Stat(j.Path)
		if err != nil {
			t.Fatalf("stat original %s: %v", j.Path, err)
		}
		if fi.Size() != 1000 && fi.Size() != 2000 {
			t.Errorf("tamanho inesperado após reject: %d", fi.Size())
		}
	}
}

func TestHandleApproveAllEmpty(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/staging/approve-all")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got approveAllResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Approved != 0 {
		t.Errorf("approved = %d, want 0", got.Approved)
	}
	if got.Errors == nil {
		t.Error("errors não deveria ser nil (deve ser [] quando vazio)")
	}
}
