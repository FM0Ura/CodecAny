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

	// StatusAwaitingApproval marca um Job cuja conversão terminou (integridade
	// OK, economia de espaço suficiente) mas que ainda não foi substituído no
	// lugar do original porque a regra/default não habilita auto_approve. O
	// output convertido permanece em staging até ApproveJob/RejectJob decidir.
	StatusAwaitingApproval JobStatus = "AWAITING_APPROVAL"
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

	// SubtitleCodecs lista, na mesma ordem dos streams de legenda EMBUTIDOS
	// no container de origem (não inclui os arquivos avulsos abaixo), o
	// codec_name reportado pelo ffprobe (ex.: "subrip", "ass", "webvtt" para
	// legendas de texto; "hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle"
	// para legendas baseadas em imagem). Usado para decidir, ao gerar MP4,
	// quais streams podem virar mov_text e quais precisam ser descartados.
	SubtitleCodecs []string `json:"subtitle_codecs,omitempty"`

	SubtitlePaths []string `json:"subtitle_paths"`
	HasVideo      bool     `json:"has_video"`
	HasAudio      bool     `json:"has_audio"`
	DurationSec   float64  `json:"duration_sec"`

	// FrameRate é o quadros/segundo do stream de vídeo real (r_frame_rate,
	// com fallback para avg_frame_rate), usado como reserva para o cálculo
	// de progresso do transcode (ver transcode.go/readProgress) quando o
	// ffmpeg reporta "out_time"/"out_time_ms" como "N/A" em -progress
	// pipe:1 — observado em containers com múltiplos streams de saída
	// (vídeo+capa+múltiplas faixas de áudio+legendas), onde "frame=" segue
	// avançando normalmente mas o timestamp de saída nunca fica disponível.
	// Zero quando não foi possível determinar (progresso então cai só no
	// "1.0" final de progress=end, mesmo degrade seguro de antes).
	FrameRate float64 `json:"frame_rate,omitempty"`

	// VideoStreamIndex é o índice do stream de vídeo "real" escolhido pelo
	// Probe entre TODOS os streams de tipo vídeo do container (ou seja, o
	// índice usado pelo seletor ffmpeg "0:v:N"), ignorando streams marcados
	// como capa/thumbnail (disposition.attached_pic). Só é significativo
	// quando HasVideo é true.
	VideoStreamIndex int `json:"video_stream_index"`

	// CoverArtStreamIndexes lista os índices (entre os streams de tipo
	// vídeo, mesmo espaço de índices de VideoStreamIndex) dos streams
	// marcados como attached_pic (capa de álbum, thumbnail embutido etc).
	// Esses streams devem ser preservados via "copy" e nunca recodificados
	// com os parâmetros (CRF/preset) do vídeo real.
	CoverArtStreamIndexes []int `json:"cover_art_stream_indexes,omitempty"`
}

// TargetSpec descreve o alvo de conversão extraído das regras (RF03).
// O driver adapter é responsável por traduzir este spec em flags nativas.
type TargetSpec struct {
	VideoCodec    string `json:"video_codec,omitempty" yaml:"codec,omitempty"`
	VideoCRF      int    `json:"video_crf,omitempty" yaml:"crf,omitempty"`
	VideoPreset   string `json:"video_preset,omitempty" yaml:"preset,omitempty"`
	VideoLossless bool   `json:"video_lossless,omitempty" yaml:"lossless,omitempty"`
	VideoHWAccel  string `json:"video_hwaccel,omitempty" yaml:"hwaccel,omitempty"`

	// VideoMaxHeight, quando > 0, limita a altura do vídeo de saída: se a
	// altura de origem for maior, o transcoder aplica um filtro de downscale
	// (ver buildArgs em pkg/adapters/ffmpeg/transcode.go). Só existe como
	// override por regra (ConvertSpec.Video.MaxHeight) — não há default
	// global para este campo, seguindo o exemplo de uso do doc de propostas.
	VideoMaxHeight int    `json:"video_max_height,omitempty" yaml:"max_height,omitempty"`
	AudioCodec     string `json:"audio_codec,omitempty" yaml:"audio_codec,omitempty"`
	AudioBitrate   string `json:"audio_bitrate,omitempty" yaml:"audio_bitrate,omitempty"`
	Container      string `json:"container,omitempty" yaml:"container,omitempty"`

	// AutoApprove indica se o Job pode ser commitado automaticamente ao final
	// da verificação de integridade (comportamento histórico, pré-v1.2) ou se
	// deve pausar em StatusAwaitingApproval até aprovação manual (default a
	// partir da v1.2 — ver mergeSpec em rules.go). Persistido na coluna JSON
	// `target` já existente, sem precisar de migração de schema.
	AutoApprove bool `json:"auto_approve,omitempty" yaml:"auto_approve,omitempty"`
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
	ID        string     `json:"id"`
	Path      string     `json:"path"`
	Status    JobStatus  `json:"status"`
	Driver    string     `json:"driver"`
	Target    TargetSpec `json:"target"`
	MediaInfo MediaInfo  `json:"media_info"`

	// OutputMediaInfo carrega os metadados medidos do arquivo GERADO pela
	// conversão (probe feito após a verificação de integridade, antes do
	// Commit/staging ser consumido — ver runJob em engine.go). nil para
	// jobs que ainda não passaram por essa etapa (QUEUED/IN_PROGRESS/
	// FAILED/ROLLED_BACK) ou cujo probe falhou.
	OutputMediaInfo *MediaInfo `json:"output_media_info,omitempty"`

	Priority    int         `json:"priority"`
	CreatedAt   time.Time   `json:"created_at"`
	StartedAt   *time.Time  `json:"started_at,omitempty"`
	FinishedAt  *time.Time  `json:"finished_at,omitempty"`
	Error       string      `json:"error,omitempty"`
	SizeMetrics SizeMetrics `json:"size_metrics"`
}
