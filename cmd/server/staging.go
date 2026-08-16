package main

import (
	"net/http"

	"github.com/FM0Ura/codecany/pkg/core"
)

// staging.go implementa os handlers da Fase B (Staging/Aprovação) — zero
// mudança em pkg/core, só handlers finos sobre Engine.ListStaged/ApproveJob/
// RejectJob/ApproveAll/RejectAll (100% prontos, ver docs/propostas_painel_controle.md,
// seção 3 "Aguardando Aprovação").

// approveAllResponse é o shape de POST /api/staging/approve-all. errors é
// sempre [] (nunca null) quando vazio — mesmo padrão de "[] em vez de null"
// já usado em handleListJobs.
type approveAllResponse struct {
	Approved int      `json:"approved"`
	Errors   []string `json:"errors"`
}

// rejectAllResponse é o shape de POST /api/staging/reject-all.
type rejectAllResponse struct {
	Rejected int      `json:"rejected"`
	Errors   []string `json:"errors"`
}

// okResponse é o shape de sucesso de approve/reject individuais.
type okResponse struct {
	OK bool `json:"ok"`
}

// registerStagingRoutes adiciona as rotas de staging ao mux — chamado a
// partir de App.routes() (router.go).
func (a *App) registerStagingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/staging", a.handleListStaging)
	mux.HandleFunc("POST /api/staging/{id}/approve", a.handleApproveJob)
	mux.HandleFunc("POST /api/staging/{id}/reject", a.handleRejectJob)
	mux.HandleFunc("POST /api/staging/approve-all", a.handleApproveAll)
	mux.HandleFunc("POST /api/staging/reject-all", a.handleRejectAll)
}

// handleListStaging atende GET /api/staging: mesmo shape de GET /api/jobs
// ([]*core.Job, [] em vez de null), filtrado a StatusAwaitingApproval.
func (a *App) handleListStaging(w http.ResponseWriter, r *http.Request) {
	jobs, err := a.Engine.ListStaged()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jobs == nil {
		jobs = []*core.Job{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

// stagedJobOr404 verifica se o job existe e está em StatusAwaitingApproval
// antes de tentar comitar/abortar o staging — traduz os dois casos de erro
// esperados de ApproveJob/RejectJob ("não encontrado" e "não está aguardando
// aprovação") para 404, sem depender de parsing de string de erro (a
// mensagem do Engine é só texto livre, não um erro sentinela). Um erro
// genuíno de I/O durante o próprio Approve/Reject (ex.: falha ao comitar o
// staging) continua virando 500 em handleApproveJob/handleRejectJob.
func (a *App) stagedJobOr404(w http.ResponseWriter, id string) bool {
	job, err := a.Engine.GetJob(id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if job == nil || job.Status != core.StatusAwaitingApproval {
		writeJSONError(w, http.StatusNotFound, "job não encontrado ou não está aguardando aprovação: "+id)
		return false
	}
	return true
}

// handleApproveJob atende POST /api/staging/{id}/approve.
func (a *App) handleApproveJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !a.stagedJobOr404(w, id) {
		return
	}
	if err := a.Engine.ApproveJob(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, okResponse{OK: true})
}

// handleRejectJob atende POST /api/staging/{id}/reject.
func (a *App) handleRejectJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !a.stagedJobOr404(w, id) {
		return
	}
	if err := a.Engine.RejectJob(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, okResponse{OK: true})
}

// handleApproveAll atende POST /api/staging/approve-all. Sucesso parcial
// (alguns jobs aprovados, outros com erro) ainda responde 200 — a UI decide
// como exibir a lista de erros (ver docs/propostas_painel_controle.md).
func (a *App) handleApproveAll(w http.ResponseWriter, r *http.Request) {
	n, errs := a.Engine.ApproveAll()
	writeJSON(w, http.StatusOK, approveAllResponse{Approved: n, Errors: errStrings(errs)})
}

// handleRejectAll atende POST /api/staging/reject-all. Mesma semântica de
// sucesso parcial de handleApproveAll.
func (a *App) handleRejectAll(w http.ResponseWriter, r *http.Request) {
	n, errs := a.Engine.RejectAll()
	writeJSON(w, http.StatusOK, rejectAllResponse{Rejected: n, Errors: errStrings(errs)})
}

// errStrings converte []error em []string, nunca nil (mesmo padrão de "[] em
// vez de null" usado no resto da API).
func errStrings(errs []error) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Error()
	}
	return out
}
