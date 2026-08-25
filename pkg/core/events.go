package core

// JobEventKind identifica o tipo do evento emitido via canal nativo (RF06, RI02).
type JobEventKind string

const (
	EventJobStart    JobEventKind = "OnJobStart"
	EventJobProgress JobEventKind = "OnJobProgress"
	EventJobComplete JobEventKind = "OnJobComplete"
	EventJobError    JobEventKind = "OnJobError"

	// EventJobAwaitingApproval é emitido quando um Job termina a verificação
	// de integridade com sucesso mas fica pausado em StatusAwaitingApproval
	// (auto_approve=false) — o output convertido está em staging aguardando
	// ApproveJob/RejectJob.
	EventJobAwaitingApproval JobEventKind = "OnJobAwaitingApproval"

	// EventJobRequeued é emitido quando um usuário reenfileira manualmente um
	// Job em StatusFailed/StatusRolledBack — prioridade máxima, próximo a
	// ser reivindicado por NextPendingJob assim que o worker atual terminar.
	EventJobRequeued JobEventKind = "OnJobRequeued"
)

// JobEvent é a unidade de comunicação emitida no canal chan JobEvent.
type JobEvent struct {
	Kind     JobEventKind `json:"kind"`
	JobID    string       `json:"job_id"`
	FilePath string       `json:"file_path"`
	Driver   string       `json:"driver_used"`
	Progress float64      `json:"progress,omitempty"`
	Success  bool         `json:"success"`
	Error    string       `json:"error,omitempty"`
	SizeDiff int64        `json:"size_diff"`

	// Metrics é preenchido em EventJobAwaitingApproval (resumo do prompt
	// interativo: tamanhos, economia, codec alvo) e em EventJobComplete
	// (tamanho original/convertido real do job recém-terminado, para
	// consumidores como o painel de controle). nil nos demais eventos
	// (Start/Progress/Error) — o tamanho convertido não existe enquanto o
	// job ainda está em voo. É *SizeMetrics (ponteiro), não SizeMetrics: em
	// Go, "omitempty" não tem efeito sobre um campo struct por valor (só
	// sobre tipos "zero-comparable" como ponteiro/slice/map/número/string),
	// então um SizeMetrics por valor seria sempre serializado no JSON —
	// mesmo zerado — inclusive em Progress, fazendo o SSE sempre carregar
	// um bloco de métricas zerado ali (bug encontrado ao testar o Dashboard
	// do painel de controle: "0 B → 0 B (0.0%)" aparecia durante o
	// progresso de todo job).
	Metrics     *SizeMetrics `json:"metrics,omitempty"`
	TargetCodec string       `json:"target_codec,omitempty"`
}
