package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/FM0Ura/codecany/pkg/core"
)

// fakeH264Prober devolve um MediaInfo que bate com a regra de fixture
// (video.codec: h264 → convert) usada por writeFixtureRules — necessário
// para exercitar Engine.RescanDirs/HandleDiscovered de ponta a ponta.
// fakeProber (router_test.go) devolve um MediaInfo zerado de propósito, o
// que nunca casaria com nenhuma regra.
type fakeH264Prober struct{}

func (fakeH264Prober) Probe(path string) (core.MediaInfo, error) {
	return core.MediaInfo{Path: path, Container: "mkv", VideoCodec: "h264"}, nil
}

func postJSON(t *testing.T, url, path string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func decodeStrings(t *testing.T, resp *http.Response) []string {
	t.Helper()
	defer resp.Body.Close()
	var out []string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode []string: %v", err)
	}
	return out
}

func TestDirsCRUD(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	dir := t.TempDir()

	// GET /api/dirs inicialmente vazio ([] em vez de null).
	resp, err := http.Get(srv.URL + "/api/dirs")
	if err != nil {
		t.Fatalf("GET /api/dirs: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := decodeStrings(t, resp); len(got) != 0 {
		t.Fatalf("esperava lista vazia, obteve %v", got)
	}

	// POST /api/dirs com diretório real e válido.
	resp = postJSON(t, srv.URL+"/api/dirs", dir)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/dirs (válido): status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// GET /api/dirs agora contém o diretório adicionado.
	resp, err = http.Get(srv.URL + "/api/dirs")
	if err != nil {
		t.Fatalf("GET /api/dirs: %v", err)
	}
	got := decodeStrings(t, resp)
	if len(got) != 1 || got[0] != filepath.Clean(dir) {
		t.Fatalf("esperava [%s], obteve %v", dir, got)
	}

	// POST duplicado → 400 (já está monitorado).
	resp = postJSON(t, srv.URL+"/api/dirs", dir)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/dirs (duplicado): status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// POST com path que é arquivo, não diretório → 400.
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp = postJSON(t, srv.URL+"/api/dirs", filePath)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/dirs (arquivo): status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// POST com path inexistente → 400.
	resp = postJSON(t, srv.URL+"/api/dirs", filepath.Join(dir, "nao-existe"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/dirs (inexistente): status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// POST com body inválido (JSON malformado) → 400.
	resp, err = http.Post(srv.URL+"/api/dirs", "application/json", bytes.NewReader([]byte("{")))
	if err != nil {
		t.Fatalf("POST /api/dirs (json malformado): %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/dirs (json malformado): status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// DELETE /api/dirs?path= sem path → 400.
	noPathReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/dirs", nil)
	if err != nil {
		t.Fatal(err)
	}
	delResp, err := http.DefaultClient.Do(noPathReq)
	if err != nil {
		t.Fatalf("DELETE /api/dirs (sem path): %v", err)
	}
	if delResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("DELETE /api/dirs (sem path): status = %d, want 400", delResp.StatusCode)
	}
	delResp.Body.Close()

	// DELETE /api/dirs?path=dir remove o diretório monitorado.
	delReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/dirs?path="+url.QueryEscape(dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	delResp, err = http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE /api/dirs: %v", err)
	}
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/dirs: status = %d, want 200", delResp.StatusCode)
	}
	delResp.Body.Close()

	// GET /api/dirs volta a ficar vazio.
	resp, err = http.Get(srv.URL + "/api/dirs")
	if err != nil {
		t.Fatalf("GET /api/dirs: %v", err)
	}
	if got := decodeStrings(t, resp); len(got) != 0 {
		t.Fatalf("esperava lista vazia após remoção, obteve %v", got)
	}
}

// TestDirsRescan cobre POST /api/dirs/rescan de ponta a ponta: registra um
// diretório com um arquivo de mídia já presente e confirma que o rescan
// enfileira um job para ele (Engine.RescanDirs → DiscoverFiles →
// HandleDiscovered).
func TestDirsRescan(t *testing.T) {
	ts := newTestServerWithProber(t, fakeH264Prober{})
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	dir := t.TempDir()
	media := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(media, make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}

	resp := postJSON(t, srv.URL+"/api/dirs", dir)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/dirs: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	rescanResp, err := http.Post(srv.URL+"/api/dirs/rescan", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/dirs/rescan: %v", err)
	}
	if rescanResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/dirs/rescan: status = %d, want 200", rescanResp.StatusCode)
	}
	rescanResp.Body.Close()

	job, err := ts.store.FindByPath(media)
	if err != nil {
		t.Fatalf("FindByPath: %v", err)
	}
	if job == nil {
		t.Fatal("esperava job enfileirado após rescan, obteve nil")
	}
	if job.Status != core.StatusQueued {
		t.Errorf("status = %v, want QUEUED", job.Status)
	}
}

func TestFsBrowse(t *testing.T) {
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.app.Handler())
	defer srv.Close()

	root := t.TempDir()
	// Árvore: root/{b,a}, root/file.txt (não deve aparecer).
	if err := os.MkdirAll(filepath.Join(root, "b-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "a-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(srv.URL + "/api/fs/browse?path=" + url.QueryEscape(root))
	if err != nil {
		t.Fatalf("GET /api/fs/browse: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got fsBrowseResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Path != filepath.Clean(root) {
		t.Errorf("path = %q, want %q", got.Path, filepath.Clean(root))
	}
	wantParent := filepath.Dir(filepath.Clean(root))
	if got.Parent != wantParent {
		t.Errorf("parent = %q, want %q", got.Parent, wantParent)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("esperava 2 entradas (só diretórios), obteve %d: %+v", len(got.Entries), got.Entries)
	}
	// Ordenado alfabeticamente por nome.
	if got.Entries[0].Name != "a-dir" || got.Entries[1].Name != "b-dir" {
		t.Errorf("ordem inesperada: %+v", got.Entries)
	}
	if got.Entries[0].Path != filepath.Join(root, "a-dir") {
		t.Errorf("path da entrada = %q, want %q", got.Entries[0].Path, filepath.Join(root, "a-dir"))
	}

	// path="" retorna a raiz "/".
	resp, err = http.Get(srv.URL + "/api/fs/browse")
	if err != nil {
		t.Fatalf("GET /api/fs/browse (default): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var rootResp fsBrowseResponse
	if err := json.NewDecoder(resp.Body).Decode(&rootResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rootResp.Path != "/" {
		t.Errorf("path default = %q, want /", rootResp.Path)
	}
	if rootResp.Parent != "" {
		t.Errorf("parent da raiz deveria ser vazio (sem \"voltar\"), obteve %q", rootResp.Parent)
	}

	// path inexistente → 400, não 500.
	resp, err = http.Get(srv.URL + "/api/fs/browse?path=" + url.QueryEscape(filepath.Join(root, "nao-existe")))
	if err != nil {
		t.Fatalf("GET /api/fs/browse (inexistente): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	// path apontando para um arquivo (não diretório) → 400.
	resp, err = http.Get(srv.URL + "/api/fs/browse?path=" + url.QueryEscape(filepath.Join(root, "file.txt")))
	if err != nil {
		t.Fatalf("GET /api/fs/browse (arquivo): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
