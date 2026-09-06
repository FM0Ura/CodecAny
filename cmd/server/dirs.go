package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/FM0Ura/codecany/pkg/core"
)

// addDirRequest é o shape do body de POST /api/dirs.
type addDirRequest struct {
	Path string `json:"path"`
}

// handleListDirs atende GET /api/dirs — lista os diretórios monitorados
// persistidos acompanhados de estatísticas de conclusão, descarte e falha.
func (a *App) handleListDirs(w http.ResponseWriter, r *http.Request) {
	stats, err := a.Engine.ListWatchedDirStats()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if stats == nil {
		// [] em vez de null no JSON — mesma convenção de handleListJobs.
		stats = []core.WatchedDirStats{}
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleAddDir atende POST /api/dirs {path}. Valida que path existe e é um
// diretório (os.Stat) e que ainda não está sendo monitorado antes de
// persistir/monitorar — 400 em qualquer caso inválido.
func (a *App) handleAddDir(w http.ResponseWriter, r *http.Request) {
	var req addDirRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "corpo inválido: "+err.Error())
		return
	}
	path := filepath.Clean(req.Path)
	if req.Path == "" {
		writeJSONError(w, http.StatusBadRequest, "path é obrigatório")
		return
	}

	fi, err := os.Stat(path)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "diretório inacessível: "+err.Error())
		return
	}
	if !fi.IsDir() {
		writeJSONError(w, http.StatusBadRequest, "path não é um diretório")
		return
	}

	existing, err := a.Engine.ListWatchedDirs()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, d := range existing {
		if d == path {
			writeJSONError(w, http.StatusBadRequest, "diretório já está sendo monitorado")
			return
		}
	}

	if err := a.Engine.AddWatchedDir(path); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// handleRemoveDir atende DELETE /api/dirs?path=... Remover um path que não
// está monitorado não é erro (mesma tolerância de Store.RemoveWatchedDir).
func (a *App) handleRemoveDir(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSONError(w, http.StatusBadRequest, "path é obrigatório")
		return
	}
	if err := a.Engine.RemoveWatchedDir(filepath.Clean(path)); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// handleRescanDirs atende POST /api/dirs/rescan — redescobre arquivos já
// presentes nos diretórios monitorados persistidos (Engine.RescanDirs) ou
// em um diretório específico quando path é informado (Engine.RescanDir).
func (a *App) handleRescanDirs(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" && r.Body != nil {
		var req struct {
			Path string `json:"path"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		path = req.Path
	}
	if path != "" {
		if err := a.Engine.RescanDir(filepath.Clean(path)); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "rescanned", "path": path})
		return
	}
	if err := a.Engine.RescanDirs(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rescanned"})
}
