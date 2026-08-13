package core

// JobEventKind identifica o tipo do evento emitido via canal nativo (RF06, RI02).
type JobEventKind string

const (
	EventJobStart    JobEventKind = "OnJobStart"
	EventJobProgress JobEventKind = "OnJobProgress"
	EventJobComplete JobEventKind = "OnJobComplete"
	EventJobError    JobEventKind = "OnJobError"
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
}
