import { useEffect } from "react";
import type { Job, MediaInfo } from "../api/types";
import { StatusChip } from "./Chip";
import { basename, formatBytes, formatDateTime, formatDuration, formatPct } from "../lib/format";
import "./JobMetadataDialog.css";

/** Uma linha da tabela comparativa Original × Gerado. */
interface MetaRow {
  label: string;
  original: string;
  output: string;
}

function resolution(m: MediaInfo): string {
  return m.width && m.height ? `${m.width}×${m.height}` : "—";
}

function videoBitrate(m: MediaInfo): string {
  return m.video_bitrate ? `${Math.round(m.video_bitrate / 1000)} kbps` : "—";
}

function audioCodecs(m: MediaInfo): string {
  return m.audio_codecs?.length ? m.audio_codecs.join(", ") : "—";
}

function frameRate(m: MediaInfo): string {
  return m.frame_rate ? `${m.frame_rate.toFixed(2)} fps` : "—";
}

const UNAVAILABLE = "Indisponível";

/** Monta as linhas da tabela comparativa a partir de media_info (sempre
 * presente) e output_media_info (pode faltar — job ainda não convertido,
 * revertido, ou anterior a esta feature). */
function buildRows(original: MediaInfo, output: MediaInfo | null | undefined): MetaRow[] {
  const o = output ?? null;
  return [
    { label: "Container", original: original.container || "—", output: o ? o.container || "—" : UNAVAILABLE },
    { label: "Codec de vídeo", original: original.video_codec || "—", output: o ? o.video_codec || "—" : UNAVAILABLE },
    { label: "Resolução", original: resolution(original), output: o ? resolution(o) : UNAVAILABLE },
    { label: "Bitrate de vídeo", original: videoBitrate(original), output: o ? videoBitrate(o) : UNAVAILABLE },
    { label: "Codecs de áudio", original: audioCodecs(original), output: o ? audioCodecs(o) : UNAVAILABLE },
    { label: "Duração", original: formatDuration(original.duration_sec), output: o ? formatDuration(o.duration_sec) : UNAVAILABLE },
    { label: "Frame rate", original: frameRate(original), output: o ? frameRate(o) : UNAVAILABLE },
  ];
}

export interface JobMetadataDialogProps {
  job: Job;
  onClose: () => void;
  /** Quando fornecidas (só na tela de Aprovação), renderiza os botões
   * Aprovar/Rejeitar no rodapé do painel. */
  onApprove?: () => void;
  onReject?: () => void;
  /** Quando fornecida (só para jobs FAILED/ROLLED_BACK, ver Fila), renderiza
   * o botão Reenfileirar no rodapé do painel. Nunca coexiste com
   * onApprove/onReject — um job não fica AWAITING_APPROVAL e FAILED/
   * ROLLED_BACK ao mesmo tempo. */
  onRequeue?: () => void;
  /** Quando fornecida (jobs em QUEUED, IN_PROGRESS ou TESTING), renderiza
   * o botão Cancelar. */
  onCancel?: () => void;
  busy?: boolean;
  actionError?: string;
}

export function JobMetadataDialog({ job, onClose, onApprove, onReject, onRequeue, onCancel, busy, actionError }: JobMetadataDialogProps) {
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  const t = job.target;
  const rows = buildRows(job.media_info, job.output_media_info);
  const showActions = Boolean(onApprove || onReject || onRequeue || onCancel);

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
          <h3>Metadados: original vs. gerado</h3>
          <div className="job-meta-compare__wrap">
            <table className="job-meta-compare">
              <thead>
                <tr>
                  <th>Atributo</th>
                  <th>Original</th>
                  <th>Gerado</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={r.label}>
                    <td>{r.label}</td>
                    <td className="font-mono">{r.original}</td>
                    <td className={`font-mono${r.output === UNAVAILABLE ? " job-meta-compare__unavailable" : ""}`}>
                      {r.output}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
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
            <h3>{job.status === "IGNORED" ? "Motivo do descarte" : "Erro"}</h3>
            <p className="job-drawer__error">{job.error}</p>
          </section>
        ) : null}

        {showActions ? (
          <section className="job-drawer__section job-drawer__actions">
            <div className="approval-table__actions">
              {onApprove ? (
                <button
                  type="button"
                  className="approval__row-btn approval__row-btn--approve"
                  disabled={busy}
                  onClick={onApprove}
                >
                  Aprovar
                </button>
              ) : null}
              {onReject ? (
                <button
                  type="button"
                  className="approval__row-btn approval__row-btn--reject"
                  disabled={busy}
                  onClick={onReject}
                >
                  Rejeitar
                </button>
              ) : null}
              {onRequeue ? (
                <button
                  type="button"
                  className="approval__row-btn approval__row-btn--requeue"
                  disabled={busy}
                  onClick={onRequeue}
                >
                  Reenfileirar
                </button>
              ) : null}
              {onCancel ? (
                <button
                  type="button"
                  className="approval__row-btn approval__row-btn--reject"
                  disabled={busy}
                  onClick={onCancel}
                >
                  Cancelar Job
                </button>
              ) : null}
            </div>
            {actionError ? <span className="approval-table__row-error">{actionError}</span> : null}
          </section>
        ) : null}
      </aside>
    </div>
  );
}
