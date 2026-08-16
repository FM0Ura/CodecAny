package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/FM0Ura/codecany/pkg/core"
)

// markerVerifier implementa core.MediaVerifier sem depender de um ffmpeg
// real: lê o conteúdo do arquivo e considera corrompido qualquer arquivo
// cujo conteúdo seja exatamente corruptMarker — suficiente para exercitar
// POST /api/health-check com arquivos reais em tempdir (um válido, um
// corrompido), mesmo espírito de fakeVerifier em router_test.go, mas capaz
// de diferenciar por path/conteúdo em vez de sempre responder nil.
type markerVerifier struct {
	corruptMarker []byte
}

func (m markerVerifier) Verify(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if bytes.Equal(data, m.corruptMarker) {
		return errors.New("erro de decodificação simulado")
	}
	return nil
}

func postHealthCheck(t *testing.T, srv *httptest.Server, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	resp, err := http.Post(srv.URL+"/api/health-check", "application/json", &buf)
	if err != nil {
		t.Fatalf("POST /api/health-check: %v", err)
	}
	return resp
}

// TestHandleHealthCheckWithRequestDirsAndFiles cobre o caso central do
// endpoint: dirs+files reais em tempdir, um arquivo válido e um corrompido,
// informados explicitamente no corpo do request (sem depender de a.Dirs).
func TestHandleHealthCheckWithRequestDirsAndFiles(t *testing.T) {
	ts := newTestServer(t)
	ts.app.Verifier = markerVerifier{corruptMarker: []byte("CORRUPT")}

	dir := t.TempDir()
	okInDir := filepath.Join(dir, "movie-ok.mkv")
	if err := os.WriteFile(okInDir, []byte("dados-validos"), 0o600); err != nil {
		t.Fatal(err)
	}
	badInDir := filepath.Join(dir, "movie-bad.mkv")
	if err := os.WriteFile(badInDir, []byte("CORRUPT"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Extra file passado via "files" (fora de dirs), também válido — cobre a
	// mistura dirs+files no mesmo request.
	extraFile := filepath.Join(t.TempDir(), "extra.mp4")
	if err := os.WriteFile(extraFile, []byte("dados-validos"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postHealthCheck(t, srv, map[string]any{
		"dirs":  []string{dir},
		"files": []string{extraFile},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got []core.HealthCheckResult
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(results) = %d, want 3: %+v", len(got), got)
	}

	byPath := make(map[string]core.HealthCheckResult, len(got))
	for _, r := range got {
		byPath[r.Path] = r
	}

	if r, ok := byPath[okInDir]; !ok || r.Status != core.HealthStatusOK || r.Error != "" {
		t.Errorf("esperava %s ok sem erro; obteve %+v (presente=%v)", okInDir, r, ok)
	}
	if r, ok := byPath[badInDir]; !ok || r.Status != core.HealthStatusCorrupted || r.Error == "" {
		t.Errorf("esperava %s corrupted com erro; obteve %+v (presente=%v)", badInDir, r, ok)
	}
	if r, ok := byPath[extraFile]; !ok || r.Status != core.HealthStatusOK {
		t.Errorf("esperava %s ok; obteve %+v (presente=%v)", extraFile, r, ok)
	}
}

// TestHandleHealthCheckFallsBackToConfiguredDirs confirma que um corpo
// vazio (ou dirs/files ausentes) cai para a.Dirs — os diretórios monitorados
// configurados no boot do servidor (fallback documentado no relatório da
// Fase E, já que a persistência de diretórios da Fase C não existe neste
// branch).
func TestHandleHealthCheckFallsBackToConfiguredDirs(t *testing.T) {
	ts := newTestServer(t)
	ts.app.Verifier = markerVerifier{corruptMarker: []byte("CORRUPT")}

	dir := t.TempDir()
	okPath := filepath.Join(dir, "a.mkv")
	if err := os.WriteFile(okPath, []byte("dados-validos"), 0o600); err != nil {
		t.Fatal(err)
	}
	ts.app.Dirs = []string{dir}

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	// Corpo completamente vazio.
	resp := postHealthCheck(t, srv, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got []core.HealthCheckResult
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Path != okPath || got[0].Status != core.HealthStatusOK {
		t.Fatalf("esperava 1 resultado ok para %s (fallback a.Dirs); obteve %+v", okPath, got)
	}
}

// TestHandleHealthCheckNoDirsConfigured cobre o caso documentado no
// relatório: sem a.Dirs configurado e sem dirs/files no request, o endpoint
// responde 400 em vez de silenciosamente varrer nada.
func TestHandleHealthCheckNoDirsConfigured(t *testing.T) {
	ts := newTestServer(t)
	ts.app.Verifier = markerVerifier{corruptMarker: []byte("CORRUPT")}
	// ts.app.Dirs propositalmente vazio.

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp := postHealthCheck(t, srv, map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// TestHandleHealthCheckInvalidBody cobre corpo JSON malformado → 400.
func TestHandleHealthCheckInvalidBody(t *testing.T) {
	ts := newTestServer(t)
	ts.app.Verifier = markerVerifier{corruptMarker: []byte("CORRUPT")}

	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/health-check", "application/json", bytes.NewBufferString("{não-json"))
	if err != nil {
		t.Fatalf("POST /api/health-check: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
