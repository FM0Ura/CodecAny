import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { ApiError, addDir, browseFs, getDirs, getJobs, removeDir, rescanDirs } from "../api/client";
import type { FsBrowseResult, Job, JobStatus, WatchedDir } from "../api/types";
import { Panel } from "../components/Panel";
import { StatusChip } from "../components/Chip";
import { JobMetadataDialog } from "../components/JobMetadataDialog";
import { formatBytes, formatPct, basename } from "../lib/format";
import { IconArrowLeft, IconClose, IconFolder, IconRefresh, IconTrash } from "../components/icons";
import "./Directories.css";

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

function relPath(fullPath: string, rootDir: string): string {
  if (fullPath.startsWith(rootDir)) {
    const sub = fullPath.slice(rootDir.length).replace(/^[/\\]+/, "");
    return sub || basename(fullPath);
  }
  return basename(fullPath);
}

const DIR_STATUS_FILTERS: { value: string; label: string }[] = [
  { value: "", label: "Todos" },
  { value: "COMPLETED", label: "Concluídos" },
  { value: "IGNORED", label: "Ignorados" },
  { value: "FAILED", label: "Falhos" },
];

export function Directories() {
  const [dirs, setDirs] = useState<WatchedDir[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pendingPath, setPendingPath] = useState<string | null>(null);
  const [rescanningAll, setRescanningAll] = useState(false);
  const [rescanningDir, setRescanningDir] = useState<string | null>(null);
  const [modalOpen, setModalOpen] = useState(false);

  // Seleção e drilldown de episódios/arquivos do diretório
  const [selectedDir, setSelectedDir] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<string>("");
  const [dirJobs, setDirJobs] = useState<Job[]>([]);
  const [dirJobsLoading, setDirJobsLoading] = useState(false);
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getDirs();
      setDirs(data);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao carregar diretórios monitorados.");
    } finally {
      setLoading(false);
    }
  }, []);

  const loadDirJobs = useCallback(async (dir: string, status?: string) => {
    setDirJobsLoading(true);
    try {
      const jobs = await getJobs({ dir, status: status || undefined });
      setDirJobs(jobs);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao carregar episódios do diretório.");
    } finally {
      setDirJobsLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (selectedDir) {
      loadDirJobs(selectedDir, statusFilter);
    } else {
      setDirJobs([]);
    }
  }, [selectedDir, statusFilter, loadDirJobs]);

  async function handleRemove(path: string, e: React.MouseEvent) {
    e.stopPropagation();
    setPendingPath(path);
    try {
      await removeDir(path);
      if (selectedDir === path) {
        setSelectedDir(null);
      }
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao remover diretório.");
    } finally {
      setPendingPath(null);
    }
  }

  async function handleRescanAll() {
    setRescanningAll(true);
    try {
      await rescanDirs();
      setError(null);
      await load();
      if (selectedDir) {
        await loadDirJobs(selectedDir, statusFilter);
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao disparar rescan.");
    } finally {
      setRescanningAll(false);
    }
  }

  async function handleRescanSingle(path: string, e: React.MouseEvent) {
    e.stopPropagation();
    setRescanningDir(path);
    try {
      await rescanDirs(path);
      setError(null);
      await load();
      if (selectedDir === path) {
        await loadDirJobs(path, statusFilter);
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao escanear diretório.");
    } finally {
      setRescanningDir(null);
    }
  }

  async function handleAdded() {
    setModalOpen(false);
    await load();
  }

  const selectedJob = useMemo(
    () => (selectedJobId ? dirJobs.find((j) => j.id === selectedJobId) ?? null : null),
    [selectedJobId, dirJobs]
  );

  return (
    <div className="directories">
      {error ? (
        <div className="directories__banner" role="alert">
          <span>{error}</span>
          <button type="button" onClick={() => setError(null)} aria-label="Descartar erro">
            <IconClose width={14} height={14} />
          </button>
        </div>
      ) : null}

      <Panel
        title={
          <span className="directories__panel-title">
            Diretórios Monitorados
            <span className="directories__actions">
              <button
                type="button"
                className="btn btn--ghost"
                onClick={handleRescanAll}
                disabled={rescanningAll || loading}
              >
                <IconRefresh width={14} height={14} />
                {rescanningAll ? "Rescaneando todos…" : "Rescan Geral"}
              </button>
              <button type="button" className="btn btn--accent" onClick={() => setModalOpen(true)}>
                Adicionar diretório
              </button>
            </span>
          </span>
        }
      >
        {loading && !dirs ? (
          <span className="directories__empty-note">Carregando diretórios monitorados…</span>
        ) : dirs && dirs.length > 0 ? (
          <div className="directories__list">
            {dirs.map((d) => {
              const isSelected = selectedDir === d.path;
              const isRescanningThis = rescanningDir === d.path;
              return (
                <div
                  className={`dir-row${isSelected ? " dir-row--selected" : ""}`}
                  key={d.path}
                  onClick={() => setSelectedDir(isSelected ? null : d.path)}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      setSelectedDir(isSelected ? null : d.path);
                    }
                  }}
                  aria-pressed={isSelected}
                  aria-label={`Selecionar diretório ${d.path}`}
                >
                  <div className="dir-row__body">
                    <div className="dir-row__header-line">
                      <span className="dir-row__path numeric" title={d.path}>
                        <IconFolder width={15} height={15} className="dir-row__icon" />
                        {d.path}
                      </span>
                      <span className="dir-row__select-hint">
                        {isSelected ? "Ocultar episódios ▲" : "Ver episódios ▼"}
                      </span>
                    </div>
                    <div className="dir-row__stats">
                      <span className="dir-stat dir-stat--completed">
                        <span className="dir-stat__dot" />
                        <strong>{d.completed}</strong> concluído{d.completed === 1 ? "" : "s"}
                      </span>
                      <span className="dir-stat dir-stat--ignored">
                        <span className="dir-stat__dot" />
                        <strong>{d.ignored}</strong> ignorado{d.ignored === 1 ? "" : "s"}
                      </span>
                      <span className="dir-stat dir-stat--failed">
                        <span className="dir-stat__dot" />
                        <strong>{d.failed}</strong> falho{d.failed === 1 ? "" : "s"}
                      </span>
                      {d.queued > 0 ? (
                        <span className="dir-stat dir-stat--queued">
                          <span className="dir-stat__dot" />
                          <strong>{d.queued}</strong> em fila/andamento
                        </span>
                      ) : null}
                      <span className="dir-stat dir-stat--total">
                        Total: <strong>{d.total}</strong>
                      </span>
                    </div>
                  </div>
                  <div className="dir-row__buttons" onClick={(e) => e.stopPropagation()}>
                    <button
                      type="button"
                      className="btn btn--ghost btn--small"
                      onClick={(e) => handleRescanSingle(d.path, e)}
                      disabled={isRescanningThis || rescanningAll}
                      title="Escanear apenas esta pasta"
                      aria-label={`Escanear ${d.path}`}
                    >
                      <IconRefresh width={13} height={13} />
                      {isRescanningThis ? "Escaneando…" : "Escanear"}
                    </button>
                    <button
                      type="button"
                      className="dir-row__remove"
                      onClick={(e) => handleRemove(d.path, e)}
                      disabled={pendingPath === d.path}
                      aria-label={`Remover ${d.path}`}
                      title="Remover pasta do monitoramento"
                    >
                      <IconTrash width={16} height={16} />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <span className="directories__empty-note">
            Nenhum diretório monitorado. Clique em &ldquo;Adicionar diretório&rdquo; para começar.
          </span>
        )}
      </Panel>

      {selectedDir ? (
        <Panel
          title={
            <div className="dir-episodes__header">
              <div className="dir-episodes__title-wrap">
                <span className="dir-episodes__title">Episódios e Arquivos</span>
                <span className="dir-episodes__subtitle numeric" title={selectedDir}>
                  {selectedDir}
                </span>
              </div>
              <div className="dir-episodes__filters">
                {DIR_STATUS_FILTERS.map((f) => (
                  <button
                    key={f.value}
                    type="button"
                    className={`dir-filter-btn${statusFilter === f.value ? " dir-filter-btn--active" : ""}`}
                    onClick={() => setStatusFilter(f.value)}
                  >
                    {f.label}
                  </button>
                ))}
              </div>
            </div>
          }
        >
          {dirJobsLoading ? (
            <span className="directories__empty-note">Carregando arquivos do diretório…</span>
          ) : dirJobs.length === 0 ? (
            <span className="directories__empty-note">
              Nenhum arquivo encontrado neste diretório com o filtro selecionado.
            </span>
          ) : (
            <div className="dir-episodes__table-wrap">
              <table className="dir-episodes__table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Arquivo</th>
                    <th>Status</th>
                    <th>Regra Aplicada</th>
                    <th>Original</th>
                    <th>Convertido</th>
                    <th>Economia</th>
                    <th>Data</th>
                  </tr>
                </thead>
                <tbody>
                  {dirJobs.map((job) => (
                    <tr
                      key={job.id}
                      className="dir-episodes__row"
                      onClick={() => setSelectedJobId(job.id)}
                      role="button"
                      tabIndex={0}
                      aria-label={`Ver metadados de ${basename(job.path)}`}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") setSelectedJobId(job.id);
                      }}
                    >
                      <td className="numeric dir-episodes__id">{job.id.slice(0, 8)}</td>
                      <td className="dir-episodes__file" title={job.path}>
                        <span className="dir-episodes__filename">{basename(job.path)}</span>
                        {relPath(job.path, selectedDir) !== basename(job.path) ? (
                          <span className="dir-episodes__relpath numeric">
                            {relPath(job.path, selectedDir)}
                          </span>
                        ) : null}
                      </td>
                      <td>
                        <StatusChip status={job.status as JobStatus} />
                      </td>
                      <td className="dir-episodes__rule">
                        {job.matched_rule ? (
                          <span className="dir-episodes__rule-tag" title={job.matched_rule}>
                            {job.matched_rule}
                          </span>
                        ) : (
                          <span className="dir-episodes__rule-none">—</span>
                        )}
                      </td>
                      <td className="numeric">
                        {formatBytes(job.size_metrics?.original_size_bytes ?? 0)}
                      </td>
                      <td className="numeric">
                        {job.size_metrics?.converted_size_bytes
                          ? formatBytes(job.size_metrics.converted_size_bytes)
                          : "—"}
                      </td>
                      <td className="numeric">
                        {job.size_metrics?.saved_bytes ? (
                          <span className="dir-episodes__saved">
                            {formatBytes(job.size_metrics.saved_bytes)}
                            {job.size_metrics.compression_ratio_pct
                              ? ` (${formatPct(job.size_metrics.compression_ratio_pct)})`
                              : ""}
                          </span>
                        ) : (
                          "—"
                        )}
                      </td>
                      <td className="numeric dir-episodes__date">
                        {formatDateTime(job.finished_at ?? job.created_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Panel>
      ) : null}

      {selectedJob ? (
        <JobMetadataDialog job={selectedJob} onClose={() => setSelectedJobId(null)} />
      ) : null}

      {modalOpen ? (
        <DirBrowserModal onClose={() => setModalOpen(false)} onSelected={handleAdded} />
      ) : null}
    </div>
  );
}

interface DirBrowserModalProps {
  onClose: () => void;
  onSelected: () => void;
}

/** Quebra um caminho absoluto em segmentos clicáveis de breadcrumb — ex.:
 * "/srv/media/anime" vira [{label:"/",path:"/"}, {label:"srv",path:"/srv"},
 * {label:"media",path:"/srv/media"}, {label:"anime",path:"/srv/media/anime"}].
 * O último segmento é sempre a localização atual (não-clicável). */
function pathSegments(path: string): { label: string; path: string }[] {
  const parts = path.split("/").filter(Boolean);
  const segments = [{ label: "/", path: "/" }];
  let acc = "";
  for (const part of parts) {
    acc += "/" + part;
    segments.push({ label: part, path: acc });
  }
  return segments;
}

/**
 * Modal navegador de diretórios (decisão de produto: preferido a um campo
 * de texto puro para adicionar diretórios monitorados — ver
 * docs/propostas_painel_controle.md, item "Diretórios Monitorados"). O
 * atalho "Ir para" abaixo não contorna essa decisão: ele só chama o mesmo
 * GET /api/fs/browse (validado/confirmado pelo servidor) que um clique
 * numa subpasta chamaria — só evita ter que descer nível a nível quando o
 * caminho já é conhecido. Consome GET /api/fs/browse para listar subpastas
 * navegáveis.
 */
function DirBrowserModal({ onClose, onSelected }: DirBrowserModalProps) {
  const [result, setResult] = useState<FsBrowseResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [gotoPath, setGotoPath] = useState("");

  const load = useCallback(async (path: string): Promise<boolean> => {
    setLoading(true);
    try {
      const data = await browseFs(path);
      setResult(data);
      setError(null);
      return true;
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao navegar no filesystem.");
      return false;
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load("");
  }, [load]);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  async function handleGoto(e: FormEvent) {
    e.preventDefault();
    const target = gotoPath.trim();
    if (!target) return;
    if (await load(target)) setGotoPath("");
  }

  async function handleSelect() {
    if (!result) return;
    setSubmitting(true);
    try {
      await addDir(result.path);
      onSelected();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao adicionar diretório.");
      setSubmitting(false);
    }
  }

  return (
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions
    <div className="dir-modal__overlay" onClick={onClose}>
      <div
        className="dir-modal"
        role="dialog"
        aria-modal="true"
        aria-label="Selecionar diretório"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="dir-modal__header">
          <span className="dir-modal__title">Selecionar diretório</span>
          <button type="button" className="dir-modal__close" onClick={onClose} aria-label="Fechar">
            <IconClose width={16} height={16} />
          </button>
        </div>

        <nav className="dir-modal__breadcrumbs numeric" aria-label="Caminho atual">
          {pathSegments(result?.path ?? "/").map((seg, i, arr) => {
            const isLast = i === arr.length - 1;
            return (
              <span key={seg.path}>
                {i > 0 ? <span className="dir-modal__crumb-sep">›</span> : null}
                <button
                  type="button"
                  className={`dir-modal__crumb${isLast ? " dir-modal__crumb--current" : ""}`}
                  onClick={() => load(seg.path)}
                  disabled={isLast || loading}
                >
                  {seg.label}
                </button>
              </span>
            );
          })}
        </nav>

        <form className="dir-modal__goto" onSubmit={handleGoto}>
          <input
            type="text"
            className="dir-modal__goto-input"
            placeholder="Ir para um caminho específico…"
            value={gotoPath}
            onChange={(e) => setGotoPath(e.target.value)}
            disabled={loading}
            aria-label="Ir para um caminho específico"
          />
          <button type="submit" className="btn btn--ghost" disabled={loading || !gotoPath.trim()}>
            Ir
          </button>
        </form>

        {error ? (
          <div className="dir-modal__error" role="alert">
            {error}
          </div>
        ) : null}

        <div className="dir-modal__list" aria-busy={loading}>
          {result && result.entries.length > 0 ? (
            result.entries.map((entry) => (
              <button
                type="button"
                key={entry.path}
                className="dir-modal__entry"
                onClick={() => load(entry.path)}
                disabled={loading}
              >
                <IconFolder width={16} height={16} />
                <span className="dir-modal__entry-name">{entry.name}</span>
              </button>
            ))
          ) : loading ? (
            <span className="directories__empty-note">Carregando…</span>
          ) : (
            <span className="directories__empty-note">Nenhuma subpasta aqui.</span>
          )}
        </div>

        <div className="dir-modal__footer">
          <button
            type="button"
            className="btn btn--ghost"
            onClick={() => result?.parent && load(result.parent)}
            disabled={!result?.parent || loading}
          >
            <IconArrowLeft width={16} height={16} />
            Voltar
          </button>
          <button
            type="button"
            className="btn btn--accent"
            onClick={handleSelect}
            disabled={loading || submitting || !result}
          >
            {submitting ? "Adicionando…" : "Selecionar este diretório"}
          </button>
        </div>
      </div>
    </div>
  );
}
