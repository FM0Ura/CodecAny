package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleGetLogs_EmptyWhenNoFile(t *testing.T) {
	app := &App{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)

	app.handleGetLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava status 200, obteve %d", rec.Code)
	}

	var resp LogsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("erro ao decodificar JSON: %v", err)
	}

	if resp.Count != 0 || len(resp.Lines) != 0 {
		t.Fatalf("esperava 0 linhas quando sem arquivo, obteve %d", resp.Count)
	}
}

func TestHandleGetLogs_ReadsFileLines(t *testing.T) {
	_ = os.MkdirAll("logs", 0o755)
	logFile := filepath.Join("logs", "codecany.log")
	content := `{"time":"2026-09-06T10:00:00Z","level":"info","msg":"iniciando codecany"}
{"time":"2026-09-06T10:01:00Z","level":"warn","msg":"aviso de teste"}
`
	_ = os.WriteFile(logFile, []byte(content), 0o644)
	defer os.Remove(logFile)

	app := &App{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logs?lines=10", nil)

	app.handleGetLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava status 200, obteve %d", rec.Code)
	}

	var resp LogsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("erro ao decodificar JSON: %v", err)
	}

	if resp.Count != 2 {
		t.Fatalf("esperava 2 linhas, obteve %d", resp.Count)
	}

	if resp.Lines[0].Level != "INFO" || resp.Lines[0].Msg != "iniciando codecany" {
		t.Fatalf("primeira linha incorreta: %+v", resp.Lines[0])
	}

	if resp.Lines[1].Level != "WARN" || resp.Lines[1].Msg != "aviso de teste" {
		t.Fatalf("segunda linha incorreta: %+v", resp.Lines[1])
	}
}
