package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// LogEntry representa uma entrada de log estruturada retornada pela API.
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Msg     string `json:"msg"`
	Raw     string `json:"raw"`
	Source  string `json:"source,omitempty"`
	JobId   string `json:"job_id,omitempty"`
	Path    string `json:"path,omitempty"`
}

// LogsResponse é o envelope de resposta para GET /api/logs.
type LogsResponse struct {
	Lines []LogEntry `json:"lines"`
	Count int        `json:"count"`
}

// handleGetLogs atende GET /api/logs?lines=100.
func (a *App) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if lStr := r.URL.Query().Get("lines"); lStr != "" {
		if val, err := strconv.Atoi(lStr); err == nil && val > 0 {
			if val > 1000 {
				val = 1000
			}
			limit = val
		}
	}

	logPath := filepath.Join("logs", "codecany.log")
	file, err := os.Open(logPath)
	if err != nil {
		// Se o arquivo ainda não existe, retorna lista vazia com status 200
		writeJSON(w, http.StatusOK, LogsResponse{
			Lines: []LogEntry{},
			Count: 0,
		})
		return
	}
	defer file.Close()

	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" {
			allLines = append(allLines, text)
		}
	}

	// Pega apenas as últimas N linhas
	start := 0
	if len(allLines) > limit {
		start = len(allLines) - limit
	}
	tail := allLines[start:]

	entries := make([]LogEntry, 0, len(tail))
	for _, raw := range tail {
		entry := parseLogLine(raw)
		entries = append(entries, entry)
	}

	writeJSON(w, http.StatusOK, LogsResponse{
		Lines: entries,
		Count: len(entries),
	})
}

// parseLogLine tenta decodificar uma linha slog JSON ou constrói fallback de texto simples.
func parseLogLine(raw string) LogEntry {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		entry := LogEntry{
			Raw: raw,
		}
		if t, ok := parsed["time"].(string); ok {
			entry.Time = t
		} else {
			entry.Time = time.Now().UTC().Format(time.RFC3339)
		}
		if lvl, ok := parsed["level"].(string); ok {
			entry.Level = strings.ToUpper(lvl)
		} else {
			entry.Level = "INFO"
		}
		if msg, ok := parsed["msg"].(string); ok {
			entry.Msg = msg
		} else if msg, ok := parsed["message"].(string); ok {
			entry.Msg = msg
		}
		if jid, ok := parsed["job_id"].(string); ok {
			entry.JobId = jid
		}
		if p, ok := parsed["path"].(string); ok {
			entry.Path = p
		}
		if src, ok := parsed["source"].(string); ok {
			entry.Source = src
		}
		return entry
	}

	return LogEntry{
		Time:  time.Now().UTC().Format(time.RFC3339),
		Level: "INFO",
		Msg:   raw,
		Raw:   raw,
	}
}
