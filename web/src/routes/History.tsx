import { useEffect, useMemo, useState } from "react";
import { ApiError, getJobs } from "../api/client";
import type { Job, JobStatus } from "../api/types";
import { Panel } from "../components/Panel";
import { StatusChip } from "../components/Chip";
import { JobMetadataDialog } from "../components/JobMetadataDialog";
import { formatBytes, formatPct, basename } from "../lib/format";
import "./History.css";

const STATUS_OPTIONS: { value: string; label: string }[] = [
  { value: "", label: "Todos os status" },
  { value: "COMPLETED", label: "Concluído" },
  { value: "FAILED", label: "Falhou" },
  { value: "ROLLED_BACK", label: "Revertido" },
  { value: "AWAITING_APPROVAL", label: "Aguardando aprovação" },
  { value: "QUEUED", label: "Na fila" },
  { value: "IN_PROGRESS", label: "Em progresso" },
];

// since é uma DURAÇÃO relativa (mesmo contrato de GET /api/jobs?since=,
// ver cmd/server/router.go::handleListJobs — time.ParseDuration, não uma
// data absoluta). Go não tem unidade "dia"; 7/30/90 dias são expressos em
// horas.
const PERIOD_OPTIONS: { value: string; label: string }[] = [
  { value: "", label: "Todo o período" },
  { value: "24h", label: "Últimas 24h" },
  { value: "168h", label: "Últimos 7 dias" },
  { value: "720h", label: "Últimos 30 dias" },
  { value: "2160h", label: "Últimos 90 dias" },
];

function formatDateTime(iso?: string): string {
  if (!iso) return "-";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "-";
  return d.toLocaleString("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    year: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** Chave de bucket diário (YYYY-MM-DD, fuso local) a partir de um ISO date. */
function dayKey(iso?: string): string | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

const DAY_LABEL_FMT = new Intl.DateTimeFormat("pt-BR", { day: "2-digit", month: "2-digit" });

const MAX_CHART_DAYS = 14;

/**
 * Tela de Histórico (Fase E). Reaproveita GET /api/jobs (status/since/until,
 * já suportado desde a Fase A) — nenhum endpoint novo. O gráfico de economia
 * por dia faz bucketing client-side sobre os jobs retornados, já que
 * Store.SavingsByDay (item 12, opcional, da proposta) não foi implementado
 * neste branch — ver relatório da Fase E.
 */
export function History() {
  const [status, setStatus] = useState("");
  const [period, setPeriod] = useState("");
  const [jobs, setJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    getJobs({ status: status || undefined, since: period || undefined })
      .then((data) => {
        if (!cancelled) setJobs(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err instanceof ApiError ? err.message : "Falha ao carregar o histórico.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [status, period]);

  // Economia por dia: só jobs COMPLETED contam como economia realizada
  // (ROLLED_BACK manteve o arquivo original — ver item 14 da proposta —
  // mesmo que SizeMetrics tenha sido calculado antes da reversão).
  const dailySavings = useMemo(() => {
    const byDay = new Map<string, number>();
    for (const job of jobs) {
      if (job.status !== "COMPLETED") continue;
      const key = dayKey(job.finished_at ?? job.created_at);
      if (!key) continue;
      byDay.set(key, (byDay.get(key) ?? 0) + Math.max(0, job.size_metrics?.saved_bytes ?? 0));
    }
    const sortedKeys = Array.from(byDay.keys()).sort();
    const visibleKeys = sortedKeys.slice(-MAX_CHART_DAYS);
    const todayKey = dayKey(new Date().toISOString());
    const max = Math.max(1, ...visibleKeys.map((k) => byDay.get(k) ?? 0));
    return visibleKeys.map((key) => ({
      key,
      bytes: byDay.get(key) ?? 0,
      pct: ((byDay.get(key) ?? 0) / max) * 100,
      isToday: key === todayKey,
      label: DAY_LABEL_FMT.format(new Date(key + "T00:00:00")),
    }));
  }, [jobs]);

  const totalSaved = useMemo(
    () => jobs.reduce((acc, j) => acc + Math.max(0, j.size_metrics?.saved_bytes ?? 0), 0),
    [jobs],
  );

  const selectedJob = jobs.find((j) => j.id === selectedId) ?? null;

  return (
    <div className="history">
      <Panel>
        <div className="history__filters">
          <label className="history__filter">
            <span>Status</span>
            <select value={status} onChange={(e) => setStatus(e.target.value)}>
              {STATUS_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </label>
          <label className="history__filter">
            <span>Período</span>
            <select value={period} onChange={(e) => setPeriod(e.target.value)}>
              {PERIOD_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </label>
          <span className="history__count numeric">
            {loading ? "carregando…" : `${jobs.length} job${jobs.length === 1 ? "" : "s"}`}
          </span>
        </div>
      </Panel>

      {error ? (
        <div className="history__banner" role="alert">
          {error}
        </div>
      ) : null}

      {dailySavings.length > 0 ? (
        <Panel title="Economia por dia">
          <div className="savings-chart">
            <div className="savings-chart__plot">
              {dailySavings.map((d) => (
                <div className="savings-chart__col" key={d.key}>
                  <div
                    className={`savings-chart__bar${d.isToday ? " savings-chart__bar--today" : ""}`}
                    style={{ height: `${Math.max(2, d.pct)}%` }}
                    data-value={`${formatBytes(d.bytes)} — ${d.label}`}
                    tabIndex={0}
                  />
                  <span className="savings-chart__axis-label">
                    {d.isToday ? "hoje" : d.label}
                  </span>
                </div>
              ))}
            </div>
          </div>
          <div className="savings-chart__total">
            Total economizado no período filtrado:{" "}
            <strong className="numeric">{formatBytes(totalSaved)}</strong>
          </div>
        </Panel>
      ) : null}

      <Panel title="Histórico de jobs">
        {loading && jobs.length === 0 ? (
          <span className="history__empty-note">Carregando…</span>
        ) : jobs.length === 0 ? (
          <span className="history__empty-note">Nenhum job encontrado para este filtro.</span>
        ) : (
          <div className="history__table-wrap">
            <table className="history__table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Arquivo</th>
                  <th>Status</th>
                  <th>Original</th>
                  <th>Convertido</th>
                  <th>Economia</th>
                  <th>Finalizado em</th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((job) => (
                  <tr
                    key={job.id}
                    className="history__row"
                    onClick={() => setSelectedId(job.id)}
                    tabIndex={0}
                    role="button"
                    aria-label={`Ver detalhes de ${basename(job.path)}`}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") setSelectedId(job.id);
                    }}
                  >
                    <td className="numeric">{job.id.slice(0, 8)}</td>
                    <td className="history__path" title={job.path}>
                      {basename(job.path)}
                    </td>
                    <td>
                      <StatusChip status={job.status as JobStatus} />
                    </td>
                    <td className="numeric">{formatBytes(job.size_metrics?.original_size_bytes ?? 0)}</td>
                    <td className="numeric">{formatBytes(job.size_metrics?.converted_size_bytes ?? 0)}</td>
                    <td className="numeric">
                      {formatBytes(job.size_metrics?.saved_bytes ?? 0)}
                      {job.size_metrics?.compression_ratio_pct
                        ? ` (${formatPct(job.size_metrics.compression_ratio_pct)})`
                        : ""}
                    </td>
                    <td className="numeric">{formatDateTime(job.finished_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      {selectedJob ? <JobMetadataDialog job={selectedJob} onClose={() => setSelectedId(null)} /> : null}
    </div>
  );
}
