package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/FM0Ura/codecany/pkg/core"
)

// serverVersion identifica a versão do binário nas respostas de /api/status.
// Sem infraestrutura de -ldflags no v1 (Fase A) — fica "dev" até uma fase
// futura decidir versionar o build.
const serverVersion = "dev"

// statusResponse é o shape de GET /api/status. Nomes de campo em snake_case
// EXATOS combinados com o frontend TypeScript (ver
// docs/propostas_painel_controle.md, seção 3) — não renomear sem coordenar
// com o frontend.
type statusResponse struct {
	Version   string  `json:"version"`
	UptimeS   float64 `json:"uptime_s"`
	Addr      string  `json:"addr"`
	DBPath    string  `json:"db_path"`
	RulesPath string  `json:"rules_path"`
	Workers   int     `json:"workers"`
}

// dashboardSummary é o shape de GET /api/dashboard/summary. status_counts
// usa string(core.JobStatus) como chave — ver handleDashboardSummary.
type dashboardSummary struct {
	StatusCounts map[string]int       `json:"status_counts"`
	TotalSavings core.SizeMetrics     `json:"total_savings"`
	HWAccel      []core.HWAccelStatus `json:"hwaccel"`
}

// App agrupa as dependências do roteamento HTTP da Fase A (Engine real,
// Broadcaster de SSE, configuração ativa, instante de start para uptime_s e
// o handler dos estáticos embutidos do SPA).
type App struct {
	Engine      *core.Engine
	Broadcaster *Broadcaster
	Config      ServerConfig
	StartedAt   time.Time
	Assets      http.Handler
	Log         *slog.Logger
}

// routes registra as rotas da Fase A (API somente-leitura + SSE + estáticos)
// num net/http.ServeMux puro (Go 1.22+, patterns com método) — sem router
// externo, coerente com a filosofia de dependências mínimas do projeto.
func (a *App) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", a.handleStatus)
	mux.HandleFunc("GET /api/dashboard/summary", a.handleDashboardSummary)
	mux.HandleFunc("GET /api/jobs", a.handleListJobs)
	mux.HandleFunc("GET /api/jobs/{id}", a.handleGetJob)
	mux.HandleFunc("GET /api/events", a.Broadcaster.ServeHTTP)
	mux.Handle("/", a.Assets)
	return mux
}

// Handler monta o http.Handler final (rotas + middlewares de logging e
// recover-from-panic, escritos à mão, sem dependência externa).
func (a *App) Handler() http.Handler {
	return a.recoverMiddleware(a.loggingMiddleware(a.routes()))
}

// loggingMiddleware loga método, path, status e duração de cada requisição.
func (a *App) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if a.Log != nil {
			a.Log.Info("http", "method", r.Method, "path", r.URL.Path,
				"status", sw.status, "duration_ms", time.Since(start).Milliseconds())
		}
	})
}

// statusRecorder captura o status code final para o log de acesso, já que
// http.ResponseWriter não expõe isso depois de WriteHeader.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush repassa para o ResponseWriter embutido quando ele implementa
// http.Flusher. Sem isso, embutir a interface http.ResponseWriter (em vez do
// tipo concreto) NÃO promove Flush automaticamente — a asserção de tipo
// `w.(http.Flusher)` feita pelo handler SSE (Broadcaster.ServeHTTP) falharia
// sempre que passasse por este middleware, quebrando o streaming.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// recoverMiddleware evita que um panic num handler derrube o processo
// inteiro: recupera, loga e responde 500 em JSON.
func (a *App) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if a.Log != nil {
					a.Log.Error("panic recuperado", "error", rec, "path", r.URL.Path)
				}
				writeJSONError(w, http.StatusInternalServerError, "erro interno")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleStatus atende GET /api/status.
func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{
		Version:   serverVersion,
		UptimeS:   time.Since(a.StartedAt).Seconds(),
		Addr:      a.Config.Addr,
		DBPath:    a.Config.DBPath,
		RulesPath: a.Config.RulesPath,
		Workers:   a.Config.Workers,
	})
}

// handleDashboardSummary atende GET /api/dashboard/summary, combinando
// Engine.CountJobsByStatus + Engine.GetTotalSavings + Engine.HWAccelStatus
// (itens 7/8/9 da proposta) num único shape.
func (a *App) handleDashboardSummary(w http.ResponseWriter, r *http.Request) {
	counts, err := a.Engine.CountJobsByStatus()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	statusCounts := make(map[string]int, len(counts))
	for status, n := range counts {
		statusCounts[string(status)] = n
	}

	savings, err := a.Engine.GetTotalSavings()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, dashboardSummary{
		StatusCounts: statusCounts,
		TotalSavings: savings,
		HWAccel:      a.Engine.HWAccelStatus(),
	})
}

// handleListJobs atende GET /api/jobs?status=&since=&until=. status é
// repassado direto como core.JobStatus (sem validação contra uma lista
// fechada — mesma tolerância de cmd/cli -history); since/until são duração
// relativa (mesmo padrão de cmd/cli -since: time.ParseDuration +
// time.Now().Add(-d)).
func (a *App) handleListJobs(w http.ResponseWriter, r *http.Request) {
	var filter core.JobFilter
	if s := r.URL.Query().Get("status"); s != "" {
		filter.Status = core.JobStatus(s)
	}
	if s := r.URL.Query().Get("since"); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "since inválido: "+err.Error())
			return
		}
		t := time.Now().Add(-d)
		filter.Since = &t
	}
	if s := r.URL.Query().Get("until"); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "until inválido: "+err.Error())
			return
		}
		t := time.Now().Add(-d)
		filter.Until = &t
	}

	jobs, err := a.Engine.ListJobs(filter)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jobs == nil {
		// [] em vez de null no JSON — mais previsível para o frontend.
		jobs = []*core.Job{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

// handleGetJob atende GET /api/jobs/{id}. Engine.GetJob retorna (nil, nil)
// quando o job não existe (não é erro) — traduzido aqui para 404 JSON.
func (a *App) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := a.Engine.GetJob(id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if job == nil {
		writeJSONError(w, http.StatusNotFound, "job não encontrado")
		return
	}
	writeJSON(w, http.StatusOK, job)
}
