import { useEffect, useRef, useState } from "react";
import { ApiError, getJob, subscribeToEvents } from "../api/client";
import type { Job, JobEvent, JobStatus } from "../api/types";
import { Chip } from "../components/Chip";
import { Panel } from "../components/Panel";
import { StatTile } from "../components/StatTile";
import { Meter, BlockMeter } from "../components/Meter";
import { LiveDot } from "../components/TopBar";
import { JobMetadataDialog } from "../components/JobMetadataDialog";
import { useDashboardSummary } from "../context/DashboardSummaryContext";
import { basename, formatBytes, formatPct } from "../lib/format";
import "./Dashboard.css";

const STATUS_ORDER: JobStatus[] = [
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

const EVENT_CHIP: Record<JobEvent["kind"], { variant: Parameters<typeof Chip>[0]["variant"]; label: string }> = {
  OnJobStart: { variant: "progress", label: "Iniciado" },
  OnJobProgress: { variant: "progress", label: "Em progresso" },
  OnJobComplete: { variant: "completed", label: "Concluído" },
  OnJobError: { variant: "failed", label: "Falhou" },
  OnJobAwaitingApproval: { variant: "awaiting", label: "Aguardando aprovação" },
  OnJobRequeued: { variant: "queued", label: "Reenfileirado" },
};

const REFRESH_ON_KINDS = new Set<JobEvent["kind"]>([
  "OnJobComplete",
  "OnJobError",
  "OnJobAwaitingApproval",
  "OnJobRequeued",
]);

const MAX_VISIBLE_JOBS = 15;

export function Dashboard() {
  const { summary, loading, error, refresh } = useDashboardSummary();
  const [liveEvents, setLiveEvents] = useState<Record<string, JobEvent>>({});
  const [order, setOrder] = useState<string[]>([]);
  const [sseStatus, setSseStatus] = useState<"connecting" | "open" | "error">("connecting");
  const [selectedJob, setSelectedJob] = useState<Job | null>(null);
  const [selectedLoading, setSelectedLoading] = useState<string | null>(null);
  const [selectedError, setSelectedError] = useState<string | null>(null);
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;

  async function openJobDetail(jobId: string) {
    setSelectedLoading(jobId);
    setSelectedError(null);
    try {
      const job = await getJob(jobId);
      setSelectedJob(job);
    } catch (err) {
      setSelectedError(err instanceof ApiError ? err.message : "Falha ao carregar detalhes do job.");
    } finally {
      setSelectedLoading((prev) => (prev === jobId ? null : prev));
    }
  }

  useEffect(() => {
    const unsubscribe = subscribeToEvents(
      (event) => {
        setLiveEvents((prev) => ({ ...prev, [event.job_id]: event }));
        setOrder((prev) => [event.job_id, ...prev.filter((id) => id !== event.job_id)]);
        if (REFRESH_ON_KINDS.has(event.kind)) {
          refreshRef.current();
        }
      },
      (status) => setSseStatus(status),
    );
    return unsubscribe;
  }, []);

  const showInitialLoading = loading && !summary;

  return (
    <div className="dashboard">
      {error ? (
        <div className="dashboard__banner" role="alert">
          <span>{error}</span>
          <span className="dashboard__banner-actions">
            <button type="button" onClick={() => refresh()}>
              Tentar novamente
            </button>
          </span>
        </div>
      ) : null}

      {showInitialLoading ? (
        <Panel>
          <div className="dashboard__loading">Carregando resumo do painel…</div>
        </Panel>
      ) : (
        <>
          <div className="dashboard__stats">
            {STATUS_ORDER.map((status) => (
              <StatTile
                key={status}
                label={STATUS_LABEL[status]}
                value={summary?.status_counts[status] ?? 0}
                accent={status === "IN_PROGRESS"}
              />
            ))}
          </div>

          <div className="dashboard__row">
            <Panel title="Economia total">
              <div className="dashboard__stats">
                <StatTile
                  label="Original"
                  value={formatBytes(summary?.total_savings.original_size_bytes ?? 0)}
                />
                <StatTile
                  label="Convertido"
                  value={formatBytes(summary?.total_savings.converted_size_bytes ?? 0)}
                />
                <StatTile
                  label="Economizado"
                  value={formatBytes(summary?.total_savings.saved_bytes ?? 0)}
                  accent
                />
                <StatTile
                  label="Compressão"
                  value={formatPct(summary?.total_savings.compression_ratio_pct ?? 0)}
                  accent
                />
              </div>
            </Panel>

            <Panel title="Utilização de HWAccel">
              <div className="dashboard__hwaccel">
                {summary && summary.hwaccel.length > 0 ? (
                  summary.hwaccel.map((h) => (
                    <BlockMeter
                      key={h.vendor}
                      label={h.vendor}
                      total={h.limit}
                      filled={h.in_use}
                    />
                  ))
                ) : (
                  <span className="dashboard__empty-note">
                    Nenhum limite de hwaccel configurado.
                  </span>
                )}
              </div>
            </Panel>
          </div>
        </>
      )}

      {selectedError ? (
        <div className="dashboard__banner" role="alert">
          <span>{selectedError}</span>
          <span className="dashboard__banner-actions">
            <button type="button" onClick={() => setSelectedError(null)}>
              Fechar
            </button>
          </span>
        </div>
      ) : null}

      <Panel
        title={
          <span
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            }}
          >
            Jobs ativos
            <LiveDot status={sseStatus} />
          </span>
        }
      >
        {order.length === 0 ? (
          <span className="dashboard__empty-note">
            Nenhum evento recebido ainda. Jobs em progresso aparecerão aqui em tempo real.
          </span>
        ) : (
          <div className="dashboard__jobs">
            {order.slice(0, MAX_VISIBLE_JOBS).map((jobId) => {
              const event = liveEvents[jobId];
              if (!event) return null;
              const chipInfo = EVENT_CHIP[event.kind];
              return (
                <div
                  className="job-row"
                  key={jobId}
                  onClick={() => openJobDetail(jobId)}
                  tabIndex={0}
                  role="button"
                  aria-label={`Ver detalhes de ${basename(event.file_path)}`}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") openJobDetail(jobId);
                  }}
                >
                  <div className="job-row__head">
                    <span className="job-row__path" title={event.file_path}>
                      {basename(event.file_path)}
                    </span>
                    <Chip variant={chipInfo.variant} label={chipInfo.label} />
                  </div>
                  <div className="job-row__meta">
                    <span>driver: {event.driver_used || "—"}</span>
                    {event.target_codec ? <span>codec: {event.target_codec}</span> : null}
                    <span className="numeric">id: {jobId.slice(0, 8)}</span>
                  </div>
                  {event.kind === "OnJobProgress" && typeof event.progress === "number" ? (
                    <Meter label="Progresso" value={event.progress} />
                  ) : null}
                  {event.kind === "OnJobError" && event.error ? (
                    <span className="job-row__error">{event.error}</span>
                  ) : null}
                  {event.metrics ? (
                    <div className="job-row__meta">
                      <span>
                        {formatBytes(event.metrics.original_size_bytes)} →{" "}
                        {formatBytes(event.metrics.converted_size_bytes)}
                      </span>
                      <span>({formatPct(event.metrics.compression_ratio_pct)})</span>
                    </div>
                  ) : null}
                  {selectedLoading === jobId ? (
                    <span className="job-row__meta">Carregando detalhes…</span>
                  ) : null}
                </div>
              );
            })}
          </div>
        )}
      </Panel>

      {selectedJob ? <JobMetadataDialog job={selectedJob} onClose={() => setSelectedJob(null)} /> : null}
    </div>
  );
}
