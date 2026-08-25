import { useCallback, useEffect, useState, type FormEvent } from "react";
import { ApiError, addDir, browseFs, getDirs, removeDir, rescanDirs } from "../api/client";
import type { FsBrowseResult } from "../api/types";
import { Panel } from "../components/Panel";
import { IconArrowLeft, IconClose, IconFolder, IconTrash } from "../components/icons";
import "./Directories.css";

export function Directories() {
  const [dirs, setDirs] = useState<string[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pendingPath, setPendingPath] = useState<string | null>(null);
  const [rescanning, setRescanning] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);

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

  useEffect(() => {
    load();
  }, [load]);

  async function handleRemove(path: string) {
    setPendingPath(path);
    try {
      await removeDir(path);
      await load();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao remover diretório.");
    } finally {
      setPendingPath(null);
    }
  }

  async function handleRescan() {
    setRescanning(true);
    try {
      await rescanDirs();
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao disparar rescan.");
    } finally {
      setRescanning(false);
    }
  }

  async function handleAdded() {
    setModalOpen(false);
    await load();
  }

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
                onClick={handleRescan}
                disabled={rescanning || loading}
              >
                {rescanning ? "Rescaneando…" : "Rescan"}
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
            {dirs.map((d) => (
              <div className="dir-row" key={d}>
                <span className="dir-row__path numeric" title={d}>
                  {d}
                </span>
                <button
                  type="button"
                  className="dir-row__remove"
                  onClick={() => handleRemove(d)}
                  disabled={pendingPath === d}
                  aria-label={`Remover ${d}`}
                >
                  <IconTrash width={16} height={16} />
                </button>
              </div>
            ))}
          </div>
        ) : (
          <span className="directories__empty-note">
            Nenhum diretório monitorado. Clique em &ldquo;Adicionar diretório&rdquo; para começar.
          </span>
        )}
      </Panel>

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
