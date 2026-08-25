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
  /** Metadados medidos do arquivo GERADO pela conversão (não o TargetSpec
   * configurado) — preenchido só após a verificação de integridade passar;
   * ausente/null para jobs ainda não convertidos, revertidos, ou anteriores
   * a esta feature. */
  output_media_info?: MediaInfo | null;
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
  | "OnJobAwaitingApproval"
  | "OnJobRequeued";

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

/** Uma subpasta listada por GET /api/fs/browse (só diretórios). */
export interface FsBrowseEntry {
  name: string;
  path: string;
}

/**
 * Shape de GET /api/fs/browse?path=. `parent` é "" quando `path` já é a
 * raiz do filesystem (sem "voltar" possível).
 */
export interface FsBrowseResult {
  path: string;
  parent: string;
  entries: FsBrowseEntry[];
}

// ---------------------------------------------------------------------
// Regras (Fase D — pkg/core/rules.go). Nomes de campo em snake_case
// combinam 1:1 com as tags JSON de core.RuleFile/Rule/Match/ConvertSpec/etc.
// ---------------------------------------------------------------------

/**
 * Espelha core.Item: aceita um valor escalar OU uma lista na API (o backend
 * agora tem MarshalJSON — Item.go item 1 da Fase D — que serializa um único
 * valor como escalar puro, não array de 1 elemento). O frontend normaliza
 * para string[] internamente ao carregar (ver normalizeRuleFile em
 * routes/Rules.tsx) — ambas as formas são aceitas de volta pelo backend na
 * hora de desserializar (UnmarshalJSON aceita escalar ou lista), então
 * sempre reenviar como array é seguro.
 */
export type RuleItem = string | string[];

export interface MatchVideo {
  codec: RuleItem;
  min_height: number;
  max_height: number;
  min_bitrate_kbps: number;
}

export interface MatchAudio {
  codec: RuleItem;
}

export interface Match {
  container: RuleItem;
  video: MatchVideo;
  audio: MatchAudio;
}

export type RuleAction = "convert" | "skip" | "";

export interface RuleTargetVideo {
  codec: string;
  crf: number;
  preset: string;
  lossless: boolean | null;
  hwaccel: string;
  max_height: number;
}

export interface RuleTargetAudio {
  codec: string;
  bitrate: string;
}

export interface ConvertSpec {
  video: RuleTargetVideo | null;
  audio: RuleTargetAudio | null;
  container: string;
}

export interface Rule {
  name: string;
  /** null/undefined = habilitada (retrocompat); só `false` desabilita. */
  enabled: boolean | null;
  match: Match;
  action: RuleAction;
  convert: ConvertSpec;
  /** Override por regra do default global de auto-aprovação. */
  auto_approve: boolean | null;
}

export interface RuleDefaults {
  video: {
    codec: string;
    crf: number;
    preset: string;
    lossless: boolean | null;
    hwaccel: string;
  };
  audio: {
    codec: string;
    bitrate: string;
  };
  container: string;
  auto_approve: boolean | null;
}

export interface SpaceSavingPolicy {
  min_saving_pct: number;
  fallback_action: string;
}

export interface WebhookConfig {
  webhook_url: string;
  events: string[] | null;
}

export interface RuleFileGlobal {
  default_driver: string;
  staging_dir: string;
  space_saving: SpaceSavingPolicy;
  defaults: RuleDefaults;
  notifications: WebhookConfig;
  hwaccel_limits: Record<string, number> | null;
}

export interface IgnoreRules {
  dir_contains: string[] | null;
  file_suffix: string[] | null;
  min_size_bytes: number;
}

export interface RuleFile {
  version: number;
  global: RuleFileGlobal;
  rules: Rule[];
  ignore: IgnoreRules;
}

export interface RuleTestRequest {
  path?: string;
  media_info?: MediaInfo;
}

export type RuleOutcome = "convert" | "skip" | "skip_no_rule";

export interface RuleTestResult {
  outcome: RuleOutcome;
  target_spec: TargetSpec;
  describe_miss: string;
}

/** Status de um arquivo verificado por POST /api/health-check (Fase E). */
export type HealthCheckStatus = "ok" | "corrupted";

/** Espelha core.HealthCheckResult (pkg/core/healthcheck.go). */
export interface HealthCheckResult {
  path: string;
  status: HealthCheckStatus;
  error?: string;
}
