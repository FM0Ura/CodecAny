import { useCallback, useEffect, useState } from "react";
import { ApiError, approveAll, approveJob, getStaging, rejectAll, rejectJob } from "../api/client";
import type { Job } from "../api/types";
import { Panel } from "../components/Panel";
import { JobMetadataDialog } from "../components/JobMetadataDialog";
import { basename, formatBytes, formatPct } from "../lib/format";
import "./Approval.css";

type BulkKind = "approve-all" | "reject-all";

interface BulkResult {
  kind: BulkKind;
  count: number;
  errors: string[];
}

export function Approval() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busyIds, setBusyIds] = useState<Set<string>>(new Set());
  const [rowError, setRowError] = useState<Record<string, string>>({});
  const [confirmKind, setConfirmKind] = useState<BulkKind | null>(null);
  const [bulkBusy, setBulkBusy] = useState(false);
  const [bulkResult, setBulkResult] = useState<BulkResult | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getStaging();
      setJobs(data);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao carregar jobs em staging.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  function markBusy(id: string, busy: boolean) {
    setBusyIds((prev) => {
      const next = new Set(prev);
      if (busy) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  async function handleApprove(id: string) {
    markBusy(id, true);
    setRowError((prev) => ({ ...prev, [id]: "" }));
    try {
      await approveJob(id);
      setJobs((prev) => prev.filter((j) => j.id !== id));
      setSelectedId((prev) => (prev === id ? null : prev));
      load();
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Falha ao aprovar o job.";
      setRowError((prev) => ({ ...prev, [id]: msg }));
    } finally {
      markBusy(id, false);
    }
  }

  async function handleReject(id: string) {
    markBusy(id, true);
    setRowError((prev) => ({ ...prev, [id]: "" }));
    try {
      await rejectJob(id);
      setJobs((prev) => prev.filter((j) => j.id !== id));
      setSelectedId((prev) => (prev === id ? null : prev));
      load();
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Falha ao rejeitar o job.";
      setRowError((prev) => ({ ...prev, [id]: msg }));
    } finally {
      markBusy(id, false);
    }
  }

  async function confirmBulk() {
    if (!confirmKind) return;
    setBulkBusy(true);
    try {
      if (confirmKind === "approve-all") {
        const res = await approveAll();
        setBulkResult({ kind: confirmKind, count: res.approved, errors: res.errors });
      } else {
        const res = await rejectAll();
        setBulkResult({ kind: confirmKind, count: res.rejected, errors: res.errors });
      }
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha na ação em lote.");
    } finally {
      setBulkBusy(false);
      setConfirmKind(null);
    }
  }

  const selectedJob = jobs.find((j) => j.id === selectedId) ?? null;

  return (
    <div className="approval">
      {error ? (
        <div className="approval__banner" role="alert">
          <span>{error}</span>
          <button type="button" onClick={() => load()}>
            Tentar novamente
          </button>
        </div>
      ) : null}

      {bulkResult ? (
        <div
          className={`approval__banner${bulkResult.errors.length ? " approval__banner--warn" : " approval__banner--ok"}`}
          role="status"
        >
          <span>
            {bulkResult.kind === "approve-all" ? "Aprovados" : "Rejeitados"}: {bulkResult.count}.
            {bulkResult.errors.length ? ` Erros: ${bulkResult.errors.length} — ${bulkResult.errors.join("; ")}` : ""}
          </span>
          <button type="button" onClick={() => setBulkResult(null)}>
            Fechar
          </button>
        </div>
      ) : null}

      <Panel
        title={
          <span className="approval__panel-head">
            Aguardando aprovação
            <span className="approval__bulk-actions">
              <button
                type="button"
                className="approval__bulk-btn approval__bulk-btn--approve"
                disabled={jobs.length === 0 || bulkBusy}
                onClick={() => setConfirmKind("approve-all")}
              >
                Aprovar todos
              </button>
              <button
                type="button"
                className="approval__bulk-btn approval__bulk-btn--reject"
                disabled={jobs.length === 0 || bulkBusy}
                onClick={() => setConfirmKind("reject-all")}
              >
                Rejeitar todos
              </button>
            </span>
          </span>
        }
      >
        {loading && jobs.length === 0 ? (
          <div className="approval__empty-note">Carregando jobs em staging…</div>
        ) : jobs.length === 0 ? (
          <div className="approval__empty-note">
            Nenhum job aguardando aprovação no momento.
          </div>
        ) : (
          <div className="approval__table-wrap">
            <table className="approval-table">
              <thead>
                <tr>
                  <th>Arquivo</th>
                  <th>Codec alvo</th>
                  <th>Original</th>
                  <th>Convertido</th>
                  <th>Economia</th>
                  <th>Ações</th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((job) => {
                  const busy = busyIds.has(job.id);
                  const rowErr = rowError[job.id];
                  return (
                    <tr
                      key={job.id}
                      className="approval-table__row"
                      onClick={() => setSelectedId(job.id)}
                      tabIndex={0}
                      role="button"
                      aria-label={`Ver detalhes de ${basename(job.path)}`}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") setSelectedId(job.id);
                      }}
                    >
                      <td className="font-mono approval-table__path" title={job.path}>
                        {basename(job.path)}
                      </td>
                      <td className="font-mono">{job.target?.video_codec || "—"}</td>
                      <td className="numeric">{formatBytes(job.size_metrics?.original_size_bytes ?? 0)}</td>
                      <td className="numeric">{formatBytes(job.size_metrics?.converted_size_bytes ?? 0)}</td>
                      <td className="numeric">
                        {formatBytes(job.size_metrics?.saved_bytes ?? 0)} (
                        {formatPct(job.size_metrics?.compression_ratio_pct ?? 0)})
                      </td>
                      <td>
                        <div className="approval-table__actions">
                          <button
                            type="button"
                            className="approval__row-btn approval__row-btn--approve"
                            disabled={busy}
                            onClick={(e) => {
                              e.stopPropagation();
                              handleApprove(job.id);
                            }}
                          >
                            Aprovar
                          </button>
                          <button
                            type="button"
                            className="approval__row-btn approval__row-btn--reject"
                            disabled={busy}
                            onClick={(e) => {
                              e.stopPropagation();
                              handleReject(job.id);
                            }}
                          >
                            Rejeitar
                          </button>
                        </div>
                        {rowErr ? <span className="approval-table__row-error">{rowErr}</span> : null}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      {confirmKind ? (
        <ConfirmDialog
          kind={confirmKind}
          count={jobs.length}
          busy={bulkBusy}
          onCancel={() => setConfirmKind(null)}
          onConfirm={confirmBulk}
        />
      ) : null}

      {selectedJob ? (
        <JobMetadataDialog
          job={selectedJob}
          onClose={() => setSelectedId(null)}
          onApprove={() => handleApprove(selectedJob.id)}
          onReject={() => handleReject(selectedJob.id)}
          busy={busyIds.has(selectedJob.id)}
          actionError={rowError[selectedJob.id]}
        />
      ) : null}
    </div>
  );
}

function ConfirmDialog({
  kind,
  count,
  busy,
  onCancel,
  onConfirm,
}: {
  kind: BulkKind;
  count: number;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !busy) onCancel();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [busy, onCancel]);

  const isApprove = kind === "approve-all";

  return (
    <div className="confirm-dialog__overlay" onClick={() => !busy && onCancel()}>
      <div
        className="confirm-dialog"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="confirm-dialog-title"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 id="confirm-dialog-title">
          {isApprove ? "Aprovar todos os jobs?" : "Rejeitar todos os jobs?"}
        </h3>
        <p>
          {isApprove
            ? `${count} arquivo(s) terão o original substituído pelo convertido, sem revisão individual. Esta ação é irreversível.`
            : `${count} arquivo(s) terão a conversão descartada e o original preservado, sem revisão individual. Esta ação é irreversível.`}
        </p>
        <div className="confirm-dialog__actions">
          <button type="button" className="confirm-dialog__cancel" onClick={onCancel} disabled={busy}>
            Cancelar
          </button>
          <button
            type="button"
            className={`confirm-dialog__confirm${isApprove ? " confirm-dialog__confirm--approve" : " confirm-dialog__confirm--reject"}`}
            onClick={onConfirm}
            disabled={busy}
          >
            {busy ? "Processando…" : isApprove ? "Aprovar todos" : "Rejeitar todos"}
          </button>
        </div>
      </div>
    </div>
  );
}
