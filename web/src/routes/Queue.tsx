import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, getJobs, subscribeToEvents } from "../api/client";
import type { Job, JobStatus } from "../api/types";
import { Panel } from "../components/Panel";
import { StatusChip } from "../components/Chip";
import { basename, formatBytes, formatDateTime, formatDuration, formatPct } from "../lib/format";
import "./Queue.css";

const STATUS_LABEL: Record<JobStatus, string> = {
  DISCOVERED: "Descoberto",
  QUEUED: "Na fila",
  IN_PROGRESS: "Em progresso",
  TESTING: "Testando",
  FINALIZING: "Finalizando",
  COMPLETED: "Concluído",
  FAILED: "Falhou",
  ROLLED_BACK: "Revertido",
  AWAITING_APPROVAL: "Aguardando aprovação",
};

const STATUS_OPTIONS: JobStatus[] = [
  "DISCOVERED",
  "QUEUED",
  "IN_PROGRESS",
  "TESTING",
  "FINALIZING",
  "AWAITING_APPROVAL",
  "COMPLETED",
  "FAILED",
  "ROLLED_BACK",
];

// Estados em que uma barra de progresso ao vivo faz sentido — os demais são
// terminais/estáticos (COMPLETED/FAILED/ROLLED_BACK/AWAITING_APPROVAL/QUEUED).
const LIVE_STATUSES = new Set<JobStatus>(["IN_PROGRESS", "TESTING", "FINALIZING"]);

// Eventos que mudam o status/métricas persistidas do job — disparam refetch
// da lista (mesmo padrão de REFRESH_ON_KINDS em routes/Dashboard.tsx).
const REFETCH_KINDS = new Set(["OnJobStart", "OnJobComplete", "OnJobError", "OnJobAwaitingApproval"]);

function InlineProgress({ value }: { value: number }) {
  const pct = Math.max(0, Math.min(1, value)) * 100;
  return (
    <div className="queue-progress">
      <div className="meter__track queue-progress__track">
        <div className="meter__fill" style={{ width: `${pct}%` }} />
      </div>
      <span className="queue-progress__value numeric">{Math.round(pct)}%</span>
    </div>
  );
}

export function Queue() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<JobStatus | "">("");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [liveProgress, setLiveProgress] = useState<Record<string, number>>({});

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getJobs(statusFilter ? { status: statusFilter } : {});
      setJobs(data);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao carregar a fila.");
    } finally {
      setLoading(false);
    }
  }, [statusFilter]);

  const loadRef = useRef(load);
  loadRef.current = load;

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    const unsubscribe = subscribeToEvents((event) => {
      if (event.kind === "OnJobProgress" && typeof event.progress === "number") {
        setLiveProgress((prev) => ({ ...prev, [event.job_id]: event.progress! }));
        return;
      }
      if (REFETCH_KINDS.has(event.kind)) {
        loadRef.current();
      }
    });
    return unsubscribe;
  }, []);

  const selectedJob = jobs.find((j) => j.id === selectedId) ?? null;

  return (
    <div className="queue">
      {error ? (
        <div className="queue__banner" role="alert">
          <span>{error}</span>
          <button type="button" onClick={() => load()}>
            Tentar novamente
          </button>
        </div>
      ) : null}

      <Panel
        title={
          <span className="queue__panel-head">
            Fila de jobs
            <label className="queue__filter">
              <span>Status</span>
              <select
                value={statusFilter}
                onChange={(e) => setStatusFilter(e.target.value as JobStatus | "")}
              >
                <option value="">Todos</option>
                {STATUS_OPTIONS.map((s) => (
                  <option key={s} value={s}>
                    {STATUS_LABEL[s]}
                  </option>
                ))}
              </select>
            </label>
          </span>
        }
      >
        {loading && jobs.length === 0 ? (
          <div className="queue__empty-note">Carregando fila…</div>
        ) : jobs.length === 0 ? (
          <div className="queue__empty-note">Nenhum job encontrado para este filtro.</div>
        ) : (
          <div className="queue__table-wrap">
            <table className="queue-table">
              <thead>
                <tr>
                  <th>Status</th>
                  <th>Arquivo</th>
                  <th>Driver</th>
                  <th>Original</th>
                  <th>Convertido</th>
                  <th>Progresso</th>
                  <th>Criado em</th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((job) => {
                  const progress = liveProgress[job.id];
                  return (
                    <tr
                      key={job.id}
                      className="queue-table__row"
                      onClick={() => setSelectedId(job.id)}
                      tabIndex={0}
                      role="button"
                      aria-label={`Ver detalhes de ${basename(job.path)}`}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") setSelectedId(job.id);
                      }}
                    >
                      <td>
                        <StatusChip status={job.status} />
                      </td>
                      <td className="queue-table__path font-mono" title={job.path}>
                        {basename(job.path)}
                      </td>
                      <td className="font-mono">{job.driver || "—"}</td>
                      <td className="numeric">{formatBytes(job.size_metrics?.original_size_bytes ?? 0)}</td>
                      <td className="numeric">
                        {job.size_metrics?.converted_size_bytes
                          ? formatBytes(job.size_metrics.converted_size_bytes)
                          : "—"}
                      </td>
                      <td>
                        {LIVE_STATUSES.has(job.status) && typeof progress === "number" ? (
                          <InlineProgress value={progress} />
                        ) : (
                          <span className="queue__empty-note">—</span>
                        )}
                      </td>
                      <td className="numeric">{formatDateTime(job.created_at)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      {selectedJob ? (
        <JobDrawer job={selectedJob} onClose={() => setSelectedId(null)} />
      ) : null}
    </div>
  );
}

function JobDrawer({ job, onClose }: { job: Job; onClose: () => void }) {
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  const m = job.media_info;
  const t = job.target;

  return (
    <div className="job-drawer__overlay" onClick={onClose}>
      <aside
        className="job-drawer"
        role="dialog"
        aria-modal="true"
        aria-label={`Detalhes do job ${basename(job.path)}`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="job-drawer__head">
          <div>
            <span className="job-drawer__path font-mono" title={job.path}>
              {basename(job.path)}
            </span>
            <StatusChip status={job.status} />
          </div>
          <button type="button" className="job-drawer__close" onClick={onClose} aria-label="Fechar">
            ×
          </button>
        </div>

        <section className="job-drawer__section">
          <h3>Identificação</h3>
          <dl className="job-drawer__dl">
            <dt>ID</dt>
            <dd className="font-mono">{job.id}</dd>
            <dt>Caminho completo</dt>
            <dd className="font-mono job-drawer__break">{job.path}</dd>
            <dt>Driver</dt>
            <dd className="font-mono">{job.driver || "—"}</dd>
          </dl>
        </section>

        <section className="job-drawer__section">
          <h3>Tamanho</h3>
          <dl className="job-drawer__dl">
            <dt>Original</dt>
            <dd className="numeric">{formatBytes(job.size_metrics?.original_size_bytes ?? 0)}</dd>
            <dt>Convertido</dt>
            <dd className="numeric">
              {job.size_metrics?.converted_size_bytes ? formatBytes(job.size_metrics.converted_size_bytes) : "—"}
            </dd>
            <dt>Economia</dt>
            <dd className="numeric">
              {job.size_metrics?.saved_bytes ? formatBytes(job.size_metrics.saved_bytes) : "—"} (
              {formatPct(job.size_metrics?.compression_ratio_pct ?? 0)})
            </dd>
          </dl>
        </section>

        <section className="job-drawer__section">
          <h3>MediaInfo (origem)</h3>
          {m ? (
            <dl className="job-drawer__dl">
              <dt>Container</dt>
              <dd className="font-mono">{m.container || "—"}</dd>
              <dt>Codec de vídeo</dt>
              <dd className="font-mono">{m.video_codec || "—"}</dd>
              <dt>Resolução</dt>
              <dd className="numeric">{m.width && m.height ? `${m.width}×${m.height}` : "—"}</dd>
              <dt>Bitrate de vídeo</dt>
              <dd className="numeric">{m.video_bitrate ? `${Math.round(m.video_bitrate / 1000)} kbps` : "—"}</dd>
              <dt>Codecs de áudio</dt>
              <dd className="font-mono">{m.audio_codecs?.length ? m.audio_codecs.join(", ") : "—"}</dd>
              <dt>Duração</dt>
              <dd className="numeric">{formatDuration(m.duration_sec)}</dd>
              <dt>Frame rate</dt>
              <dd className="numeric">{m.frame_rate ? `${m.frame_rate.toFixed(2)} fps` : "—"}</dd>
            </dl>
          ) : (
            <span className="queue__empty-note">Sem MediaInfo.</span>
          )}
        </section>

        <section className="job-drawer__section">
          <h3>TargetSpec (alvo)</h3>
          {t ? (
            <dl className="job-drawer__dl">
              <dt>Codec de vídeo</dt>
              <dd className="font-mono">{t.video_codec || "—"}</dd>
              <dt>CRF</dt>
              <dd className="numeric">{typeof t.video_crf === "number" ? t.video_crf : "—"}</dd>
              <dt>Preset</dt>
              <dd className="font-mono">{t.video_preset || "—"}</dd>
              <dt>Lossless</dt>
              <dd>{t.video_lossless ? "Sim" : "Não"}</dd>
              <dt>HWAccel</dt>
              <dd className="font-mono">{t.video_hwaccel || "—"}</dd>
              <dt>Altura máxima</dt>
              <dd className="numeric">{t.video_max_height ? `${t.video_max_height}p` : "—"}</dd>
              <dt>Codec de áudio</dt>
              <dd className="font-mono">{t.audio_codec || "—"}</dd>
              <dt>Bitrate de áudio</dt>
              <dd className="font-mono">{t.audio_bitrate || "—"}</dd>
              <dt>Container</dt>
              <dd className="font-mono">{t.container || "—"}</dd>
              <dt>Auto-aprovação</dt>
              <dd>{t.auto_approve ? "Sim" : "Não"}</dd>
            </dl>
          ) : (
            <span className="queue__empty-note">Sem TargetSpec.</span>
          )}
        </section>

        <section className="job-drawer__section">
          <h3>Timestamps</h3>
          <dl className="job-drawer__dl">
            <dt>Criado em</dt>
            <dd className="numeric">{formatDateTime(job.created_at)}</dd>
            <dt>Iniciado em</dt>
            <dd className="numeric">{formatDateTime(job.started_at)}</dd>
            <dt>Finalizado em</dt>
            <dd className="numeric">{formatDateTime(job.finished_at)}</dd>
          </dl>
        </section>

        {job.error ? (
          <section className="job-drawer__section">
            <h3>Erro</h3>
            <p className="job-drawer__error">{job.error}</p>
          </section>
        ) : null}
      </aside>
    </div>
  );
}
