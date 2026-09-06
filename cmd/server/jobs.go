package main

import (
	"net/http"

	"github.com/FM0Ura/codecany/pkg/core"
)

// jobs.go implementa POST /api/jobs/{id}/requeue — reenfileira manualmente um
// job FAILED/ROLLED_BACK com prioridade máxima (ver Engine.RequeueJob), zero
// mudança em pkg/core, só handler fino sobre Engine.GetJob/RequeueJob, mesmo
// padrão de staging.go.

// registerJobsRoutes adiciona as rotas de ação sobre jobs individuais ao mux
// — chamado a partir de App.registerPhaseRoutes() (router.go). Distinto de
// GET /api/jobs e GET /api/jobs/{id} (somente-leitura, registradas
// diretamente em App.routes()) por ser uma ação de mutação, mesmo critério
// de organização já usado por staging.go/dirs.go/rules.go.
func (a *App) registerJobsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/jobs/{id}/requeue", a.handleRequeueJob)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", a.handleCancelJob)
	mux.HandleFunc("POST /api/queue/cancel-all", a.handleCancelAllJobs)
	mux.HandleFunc("POST /api/queue/pause", a.handlePauseQueue)
	mux.HandleFunc("POST /api/queue/resume", a.handleResumeQueue)
	mux.HandleFunc("GET /api/queue/status", a.handleQueueStatus)
}

// requeuableJobOr404 espelha stagedJobOr404 (staging.go), mas verificando
// StatusFailed/StatusRolledBack em vez de StatusAwaitingApproval.
func (a *App) requeuableJobOr404(w http.ResponseWriter, id string) bool {
	job, err := a.Engine.GetJob(id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if job == nil || (job.Status != core.StatusFailed && job.Status != core.StatusRolledBack) {
		writeJSONError(w, http.StatusNotFound, "job não encontrado ou não está FAILED/ROLLED_BACK: "+id)
		return false
	}
	return true
}

// handleRequeueJob atende POST /api/jobs/{id}/requeue.
func (a *App) handleRequeueJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !a.requeuableJobOr404(w, id) {
		return
	}
	if err := a.Engine.RequeueJob(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, okResponse{OK: true})
}

// handleCancelJob atende POST /api/jobs/{id}/cancel.
func (a *App) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.Engine.CancelJob(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, okResponse{OK: true})
}

// handleCancelAllJobs atende POST /api/queue/cancel-all.
func (a *App) handleCancelAllJobs(w http.ResponseWriter, r *http.Request) {
	count, err := a.Engine.CancelAll()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": count})
}

// handlePauseQueue atende POST /api/queue/pause.
func (a *App) handlePauseQueue(w http.ResponseWriter, r *http.Request) {
	a.Engine.PauseQueue()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "paused": true})
}

// handleResumeQueue atende POST /api/queue/resume.
func (a *App) handleResumeQueue(w http.ResponseWriter, r *http.Request) {
	a.Engine.ResumeQueue()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "paused": false})
}

// handleQueueStatus atende GET /api/queue/status.
func (a *App) handleQueueStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"paused": a.Engine.IsQueuePaused()})
}
