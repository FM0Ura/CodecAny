import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, cancelAllJobs, cancelJob, getJobs, pauseQueue, requeueJob, resumeQueue, subscribeToEvents } from "../api/client";
import type { Job, JobStatus } from "../api/types";
import { Panel } from "../components/Panel";
import { StatusChip } from "../components/Chip";
import { JobMetadataDialog } from "../components/JobMetadataDialog";
import { useDashboardSummary } from "../context/DashboardSummaryContext";
import { basename, formatBytes, formatDateTime } from "../lib/format";
import "./Queue.css";

const STATUS_LABEL: Record<JobStatus, string> = {
  DISCOVERED: "Descoberto",
  QUEUED: "Na fila",
  IN_PROGRESS: "Em progresso",
  TESTING: "Testando",
  FINALIZING: "Finalizando",
  COMPLETED: "Concluído",
  IGNORED: "Ignorado",
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
  "IGNORED",
  "FAILED",
  "ROLLED_BACK",
];

// Estados em que uma barra de progresso ao vivo faz sentido — os demais são
// terminais/estáticos (COMPLETED/FAILED/ROLLED_BACK/AWAITING_APPROVAL/QUEUED).
const LIVE_STATUSES = new Set<JobStatus>(["IN_PROGRESS", "TESTING", "FINALIZING"]);

// Eventos que mudam o status/métricas persistidas do job — disparam refetch
// da lista (mesmo padrão de REFRESH_ON_KINDS em routes/Dashboard.tsx).
const REFETCH_KINDS = new Set([
  "OnJobStart",
  "OnJobComplete",
  "OnJobError",
  "OnJobAwaitingApproval",
  "OnJobRequeued",
]);

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
  const { summary, refresh: refreshSummary } = useDashboardSummary();
  const [jobs, setJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<JobStatus | "">("");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [liveProgress, setLiveProgress] = useState<Record<string, number>>({});
  const [actionBusy, setActionBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

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
        refreshSummary();
      }
    });
    return unsubscribe;
  }, [refreshSummary]);

  const selectedJob = jobs.find((j) => j.id === selectedId) ?? null;

  function selectJob(id: string) {
    setActionError(null);
    setSelectedId(id);
  }

  async function handleToggleQueue() {
    setActionBusy(true);
    setActionError(null);
    try {
      if (summary?.queue_paused) {
        await resumeQueue();
      } else {
        await pauseQueue();
      }
      await refreshSummary();
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Falha ao alterar estado da fila.");
    } finally {
      setActionBusy(false);
    }
  }

  async function handleCancelAll() {
    if (!window.confirm("Deseja realmente cancelar todos os jobs em andamento e na fila?")) {
      return;
    }
    setActionBusy(true);
    setActionError(null);
    try {
      await cancelAllJobs();
      await load();
      await refreshSummary();
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Falha ao cancelar jobs.");
    } finally {
      setActionBusy(false);
    }
  }

  async function handleRequeue(id: string, e?: React.MouseEvent) {
    if (e) e.stopPropagation();
    setActionBusy(true);
    setActionError(null);
    try {
      await requeueJob(id);
      await load();
      await refreshSummary();
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Falha ao reenfileirar o job.");
    } finally {
      setActionBusy(false);
    }
  }

  async function handleCancel(id: string, e?: React.MouseEvent) {
    if (e) e.stopPropagation();
    setActionBusy(true);
    setActionError(null);
    try {
      await cancelJob(id);
      await load();
      await refreshSummary();
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Falha ao cancelar o job.");
    } finally {
      setActionBusy(false);
    }
  }

  const isPaused = Boolean(summary?.queue_paused);
  const cancellableCount = jobs.filter(
    (j) => j.status === "QUEUED" || j.status === "IN_PROGRESS" || j.status === "TESTING"
  ).length;

  return (
    <div className="queue">
      <div className="queue__toolbar">
        <div className="queue__toolbar-title">
          <span>Status da Fila:</span>
          <span
            className={`queue__queue-status-tag ${
              isPaused ? "queue__queue-status-tag--paused" : "queue__queue-status-tag--running"
            }`}
          >
            {isPaused ? "⏸ Fila Pausada" : "▶ Fila em Execução"}
          </span>
        </div>
        <div className="queue__toolbar-actions">
          <button
            type="button"
            className={`queue__queue-btn ${
              isPaused ? "queue__queue-btn--resume" : "queue__queue-btn--pause"
            }`}
            disabled={actionBusy}
            onClick={handleToggleQueue}
          >
            {actionBusy
              ? "Aguarde…"
              : isPaused
              ? "Retomar Processamento"
              : "Pausar Processamento"}
          </button>
          <button
            type="button"
            className="queue__queue-btn queue__queue-btn--cancel-all"
            disabled={actionBusy || cancellableCount === 0}
            onClick={handleCancelAll}
            title="Cancela todos os jobs que estão em andamento ou pendentes na fila"
          >
            Cancelar Todos ({cancellableCount})
          </button>
        </div>
      </div>

      {actionError ? (
        <div className="queue__banner" role="alert">
          <span>{actionError}</span>
          <button type="button" onClick={() => setActionError(null)}>
            Fechar
          </button>
        </div>
      ) : null}

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
                  <th style={{ textAlign: "right" }}>Ações</th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((job) => {
                  const progress = liveProgress[job.id];
                  const canCancel =
                    job.status === "QUEUED" || job.status === "IN_PROGRESS" || job.status === "TESTING";
                  const canRequeue = job.status === "FAILED" || job.status === "ROLLED_BACK";

                  return (
                    <tr
                      key={job.id}
                      className="queue-table__row"
                      onClick={() => selectJob(job.id)}
                      tabIndex={0}
                      role="button"
                      aria-label={`Ver detalhes de ${basename(job.path)}`}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") selectJob(job.id);
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
                      <td style={{ textAlign: "right" }} onClick={(e) => e.stopPropagation()}>
                        {canCancel ? (
                          <button
                            type="button"
                            className="queue__table-btn queue__table-btn--cancel"
                            disabled={actionBusy}
                            onClick={(e) => handleCancel(job.id, e)}
                          >
                            Cancelar
                          </button>
                        ) : canRequeue ? (
                          <button
                            type="button"
                            className="queue__table-btn queue__table-btn--requeue"
                            disabled={actionBusy}
                            onClick={(e) => handleRequeue(job.id, e)}
                          >
                            Reenfileirar
                          </button>
                        ) : (
                          <span className="queue__empty-note">—</span>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      {selectedJob ? (
        <JobMetadataDialog
          job={selectedJob}
          onClose={() => setSelectedId(null)}
          onRequeue={
            selectedJob.status === "FAILED" || selectedJob.status === "ROLLED_BACK"
              ? () => handleRequeue(selectedJob.id)
              : undefined
          }
          onCancel={
            selectedJob.status === "QUEUED" || selectedJob.status === "IN_PROGRESS" || selectedJob.status === "TESTING"
              ? () => handleCancel(selectedJob.id)
              : undefined
          }
          busy={actionBusy}
          actionError={actionError ?? undefined}
        />
      ) : null}
    </div>
  );
}
