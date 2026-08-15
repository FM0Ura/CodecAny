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

	// Metrics e TargetCodec só são preenchidos em EventJobAwaitingApproval,
	// para compor o resumo do prompt interativo (tamanhos, economia, codec
	// alvo) sem precisar consultar o Store de volta.
	Metrics     SizeMetrics `json:"metrics,omitempty"`
	TargetCodec string      `json:"target_codec,omitempty"`
}
