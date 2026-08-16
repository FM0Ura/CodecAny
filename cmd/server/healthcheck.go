package main

import (
	"encoding/json"
	"net/http"

	"github.com/FM0Ura/codecany/pkg/core"
)

// healthCheckRequest é o corpo (opcional) de POST /api/health-check. Quando
// ambos os campos vêm vazios/ausentes, o handler cai no fallback de
// Engine.ListWatchedDirs()/a.Dirs — ver handleHealthCheck.
type healthCheckRequest struct {
	Dirs  []string `json:"dirs,omitempty"`
	Files []string `json:"files,omitempty"`
}

// handleHealthCheck atende POST /api/health-check: varre dirs/files em busca
// de arquivos corrompidos via core.RunHealthCheck (item 11 da proposta,
// mesma função usada pela CLI em -health-check) e retorna
// []core.HealthCheckResult como JSON. Síncrono no v1 (sem streaming/SSE) —
// para bibliotecas grandes isso pode levar bastante tempo; uma fase futura
// pode mover para um job assíncrono reportado via SSE.
//
// Corpo vazio ou com dirs/files ausentes usa, nesta ordem: (1)
// Engine.ListWatchedDirs() — os diretórios monitorados persistidos da Fase C
// (tabela watched_dirs), consultados a cada request (não um snapshot de
// boot) para refletir alterações feitas pela tela de Diretórios sem precisar
// reiniciar o servidor; (2) a.Dirs — snapshot efêmero de -dir capturado no
// boot deste processo, só para compatibilidade com quem roda o servidor sem
// nunca configurar diretórios pelo painel. Bug corrigido aqui: antes da
// integração da Fase C neste branch, só (2) existia — health-check com corpo
// vazio nunca enxergava diretórios adicionados via painel (só via -dir), o
// que o tornava inútil pro fluxo normal de uso pelo painel. Se nenhuma das
// duas fontes tiver diretórios E o request não informar dirs/files, o
// handler responde 400: não há diretório algum para varrer.
func (a *App) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	var req healthCheckRequest
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil && err.Error() != "EOF" {
			writeJSONError(w, http.StatusBadRequest, "corpo inválido: "+err.Error())
			return
		}
	}

	dirs := req.Dirs
	files := req.Files
	if len(dirs) == 0 && len(files) == 0 {
		watched, err := a.Engine.ListWatchedDirs()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		dirs = watched
	}
	if len(dirs) == 0 && len(files) == 0 {
		dirs = a.Dirs
	}
	if len(dirs) == 0 && len(files) == 0 {
		writeJSONError(w, http.StatusBadRequest,
			"nenhum diretório monitorado configurado e nenhum dirs/files informado no request")
		return
	}

	results, err := core.RunHealthCheck(dirs, files, a.Verifier)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if results == nil {
		// [] em vez de null no JSON — mesmo padrão de handleListJobs.
		results = []core.HealthCheckResult{}
	}
	writeJSON(w, http.StatusOK, results)
}
