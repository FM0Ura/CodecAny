package core

import "time"

// JobStatus representa os estados do ciclo de vida do Job (seção 9 da spec).
type JobStatus string

const (
	StatusDiscovered JobStatus = "DISCOVERED"
	StatusQueued     JobStatus = "QUEUED"
	StatusInProgress JobStatus = "IN_PROGRESS"
	StatusTesting    JobStatus = "TESTING"
	StatusFinalizing JobStatus = "FINALIZING"
	StatusCompleted  JobStatus = "COMPLETED"
	StatusFailed     JobStatus = "FAILED"
	StatusRolledBack JobStatus = "ROLLED_BACK"
)

// MediaInfo carrega os metadados técnicos extraídos pelo MediaProber (RF02).
type MediaInfo struct {
	Path         string   `json:"path"`
	Container    string   `json:"container"`
	VideoCodec   string   `json:"video_codec"`
	VideoBitrate int64    `json:"video_bitrate"`
	Width        int      `json:"width"`
	Height       int      `json:"height"`
	AudioCodecs  []string `json:"audio_codecs"`
	SubtitleDirs []string `json:"subtitle_dirs"`
	HasVideo     bool     `json:"has_video"`
	HasAudio     bool     `json:"has_audio"`
	DurationSec  float64  `json:"duration_sec"`
}

// TargetSpec descreve o alvo de conversão extraído das regras (RF03).
// O driver adapter é responsável por traduzir este spec em flags nativas.
type TargetSpec struct {
	VideoCodec   string `json:"video_codec,omitempty" yaml:"codec,omitempty"`
	VideoCRF     int    `json:"video_crf,omitempty" yaml:"crf,omitempty"`
	VideoPreset  string `json:"video_preset,omitempty" yaml:"preset,omitempty"`
	AudioCodec   string `json:"audio_codec,omitempty" yaml:"audio_codec,omitempty"`
	AudioBitrate string `json:"audio_bitrate,omitempty" yaml:"audio_bitrate,omitempty"`
	Container    string `json:"container,omitempty" yaml:"container,omitempty"`
}

// SizeMetrics carrega as métricas de eficiência de espaço (seção 7).
type SizeMetrics struct {
	OriginalSizeBytes   int64   `json:"original_size_bytes"`
	ConvertedSizeBytes  int64   `json:"converted_size_bytes"`
	SavedBytes          int64   `json:"saved_bytes"`
	CompressionRatioPct float64 `json:"compression_ratio_pct"`
}

// Job representa um item da fila persistido no SQLite.
type Job struct {
	ID          string      `json:"id"`
	Path        string      `json:"path"`
	Status      JobStatus   `json:"status"`
	Driver      string      `json:"driver"`
	Target      TargetSpec  `json:"target"`
	MediaInfo   MediaInfo   `json:"media_info"`
	Priority    int         `json:"priority"`
	CreatedAt   time.Time   `json:"created_at"`
	StartedAt   *time.Time  `json:"started_at,omitempty"`
	FinishedAt  *time.Time  `json:"finished_at,omitempty"`
	Error       string      `json:"error,omitempty"`
	SizeMetrics SizeMetrics `json:"size_metrics"`
}
