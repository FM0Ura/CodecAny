// Tipos espelhando o contrato de API do CodecAny (ver docs/propostas_painel_controle.md).
// Nomes de campo em snake_case combinam 1:1 com as tags JSON do backend Go.

export type JobStatus =
  | "DISCOVERED"
  | "QUEUED"
  | "IN_PROGRESS"
  | "TESTING"
  | "FINALIZING"
  | "COMPLETED"
  | "FAILED"
  | "ROLLED_BACK"
  | "AWAITING_APPROVAL";

export interface TargetSpec {
  video_codec?: string;
  video_crf?: number;
  video_preset?: string;
  video_lossless?: boolean;
  video_hwaccel?: string;
  video_max_height?: number;
  audio_codec?: string;
  audio_bitrate?: string;
  container?: string;
  auto_approve?: boolean;
}

export interface MediaInfo {
  path: string;
  container: string;
  video_codec: string;
  video_bitrate: number;
  width: number;
  height: number;
  audio_codecs: string[];
  subtitle_codecs?: string[];
  subtitle_paths: string[];
  has_video: boolean;
  has_audio: boolean;
  duration_sec: number;
  frame_rate?: number;
  video_stream_index: number;
  cover_art_stream_indexes?: number[];
}

export interface SizeMetrics {
  original_size_bytes: number;
  converted_size_bytes: number;
  saved_bytes: number;
  compression_ratio_pct: number;
}

export interface Job {
  id: string;
  path: string;
  status: JobStatus;
  driver: string;
  target: TargetSpec;
  media_info: MediaInfo;
  priority: number;
  created_at: string; // RFC3339
  started_at?: string;
  finished_at?: string;
  error?: string;
  size_metrics: SizeMetrics;
}

export type JobEventKind =
  | "OnJobStart"
  | "OnJobProgress"
  | "OnJobComplete"
  | "OnJobError"
  | "OnJobAwaitingApproval";

export interface JobEvent {
  kind: JobEventKind;
  job_id: string;
  file_path: string;
  driver_used: string;
  progress?: number; // 0.0–1.0, só em OnJobProgress
  success: boolean;
  error?: string;
  size_diff: number;
  metrics?: SizeMetrics;
  target_codec?: string;
}

export interface HWAccelStatus {
  vendor: string;
  limit: number;
  in_use: number;
}

export interface DashboardSummary {
  status_counts: Partial<Record<JobStatus, number>>;
  total_savings: SizeMetrics;
  hwaccel: HWAccelStatus[];
}

export interface ServerStatus {
  version: string;
  uptime_s: number;
  addr: string;
  db_path: string;
  rules_path: string;
  workers: number;
}
