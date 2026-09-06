import { useEffect, useState } from "react";
import { ApiError, getDirs, runHealthCheck } from "../api/client";
import type { HealthCheckResult } from "../api/types";
import { Panel } from "../components/Panel";
import { StatTile } from "../components/StatTile";
import { basename } from "../lib/format";
import "./HealthCheck.css";

type ScanState = "idle" | "running" | "done" | "error";

/** Sentinela do <select> de escopo — "todos os diretórios monitorados". */
const SCOPE_ALL = "__all__";

/**
 * Tela de Health Check (Fase E, item 11 da proposta). POST /api/health-check
 * é síncrono no v1 — sem streaming/SSE — então a UI só tem três estados:
 * idle (botão + campos opcionais de escopo/dirs/files), running (indicador
 * de carregamento, sem barra de progresso real já que o servidor não
 * reporta progresso parcial) e done/error (lista de resultados ou mensagem
 * de erro).
 *
 * Escopo: dropdown alimentado por GET /api/dirs (diretórios monitorados
 * persistidos, Fase C) — "Todos" ou um diretório específico, útil em
 * bibliotecas grandes com várias pastas monitoradas onde rodar tudo de uma
 * vez é caro. O escopo escolhido é sempre resolvido para uma lista explícita
 * de dirs enviada no request (não depende do fallback do servidor quando
 * há diretórios monitorados) — assim o que aparece selecionado na tela é
 * exatamente o que é varrido. O textarea abaixo continua disponível para
 * caminhos avulsos fora dos diretórios monitorados (ex.: um arquivo solto).
 * Corpo totalmente vazio (nenhum diretório monitorado E nada no textarea)
 * cai no fallback do servidor (ver cmd/server/healthcheck.go).
 */
export function HealthCheck() {
  const [watchedDirs, setWatchedDirs] = useState<string[]>([]);
  const [scope, setScope] = useState<string>(SCOPE_ALL);
  const [pathsInput, setPathsInput] = useState("");
  const [state, setState] = useState<ScanState>("idle");
  const [results, setResults] = useState<HealthCheckResult[]>([]);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    getDirs()
      .then((data) => setWatchedDirs(data.map((d) => d.path)))
      .catch(() => {
        // Falha ao listar diretórios monitorados não impede o uso da tela —
        // só o dropdown de escopo fica limitado a "Todos" (vazio) e o
        // textarea manual continua funcionando normalmente.
      });
  }, []);

  const runScan = async () => {
    setState("running");
    setErrorMessage(null);

    const lines = pathsInput
      .split("\n")
      .map((l) => l.trim())
      .filter(Boolean);
    const manualDirs = lines.filter((l) => l.endsWith("/"));
    const files = lines.filter((l) => !l.endsWith("/"));

    const scopeDirs = scope === SCOPE_ALL ? watchedDirs : [scope];
    const dirs = Array.from(new Set([...scopeDirs, ...manualDirs]));

    try {
      const res = await runHealthCheck(
        dirs.length === 0 && files.length === 0 ? {} : { dirs, files },
      );
      setResults(res);
      setState("done");
    } catch (err) {
      const message =
        err instanceof ApiError ? err.message : "Falha ao rodar a verificação de integridade.";
      setErrorMessage(message);
      setState("error");
    }
  };

  const corrupted = results.filter((r) => r.status === "corrupted");
  const ok = results.filter((r) => r.status === "ok");

  return (
    <div className="health-check">
      <Panel title="Verificação de integridade">
        <p className="health-check__intro">
          Decodifica cada arquivo de ponta a ponta (via ffmpeg) para detectar corrupção, sem
          transcodificar nada. Pode demorar bastante em bibliotecas grandes — a verificação roda
          de forma síncrona no servidor.
        </p>

        <label className="health-check__label" htmlFor="hc-scope">
          Escopo
        </label>
        <select
          id="hc-scope"
          className="health-check__select"
          value={scope}
          onChange={(e) => setScope(e.target.value)}
          disabled={state === "running"}
        >
          <option value={SCOPE_ALL}>
            Todos os diretórios monitorados
            {watchedDirs.length > 0 ? ` (${watchedDirs.length})` : ""}
          </option>
          {watchedDirs.map((d) => (
            <option key={d} value={d}>
              {d}
            </option>
          ))}
        </select>
        {watchedDirs.length === 0 ? (
          <span className="health-check__empty-note">
            Nenhum diretório monitorado configurado (tela Diretórios) — use o campo abaixo para
            informar caminhos manualmente.
          </span>
        ) : null}

        <label className="health-check__label" htmlFor="hc-paths">
          Diretórios/arquivos adicionais (opcional — um por linha; diretórios terminam em
          &ldquo;/&rdquo;). Somados ao escopo selecionado acima.
        </label>
        <textarea
          id="hc-paths"
          className="health-check__textarea"
          placeholder={"/media/extra/\n/media/extra/arquivo-solto.mkv"}
          value={pathsInput}
          onChange={(e) => setPathsInput(e.target.value)}
          disabled={state === "running"}
          rows={4}
        />

        <div className="health-check__actions">
          <button
            type="button"
            className="health-check__run-btn"
            onClick={runScan}
            disabled={state === "running"}
          >
            {state === "running" ? "Verificando…" : "Rodar verificação"}
          </button>
          {state === "running" ? (
            <span className="health-check__spinner" role="status" aria-live="polite">
              <span className="health-check__spinner-dot" aria-hidden="true" />
              Varrendo arquivos, aguarde…
            </span>
          ) : null}
        </div>
      </Panel>

      {state === "error" && errorMessage ? (
        <div className="health-check__banner" role="alert">
          {errorMessage}
        </div>
      ) : null}

      {state === "done" ? (
        <>
          <div className="health-check__stats">
            <StatTile label="Arquivos verificados" value={results.length} />
            <StatTile label="OK" value={ok.length} accent />
            <StatTile label="Corrompidos" value={corrupted.length} />
          </div>

          <Panel title={corrupted.length > 0 ? "Arquivos corrompidos" : "Resultado"}>
            {results.length === 0 ? (
              <span className="health-check__empty-note">
                Nenhum arquivo encontrado para verificar.
              </span>
            ) : corrupted.length === 0 ? (
              <span className="health-check__ok-note">
                Tudo OK — nenhum arquivo corrompido encontrado entre {results.length}{" "}
                verificado{results.length === 1 ? "" : "s"}.
              </span>
            ) : (
              <ul className="health-check__list">
                {corrupted.map((r) => (
                  <li className="health-check__item health-check__item--corrupted" key={r.path}>
                    <span className="health-check__item-path" title={r.path}>
                      {basename(r.path)}
                    </span>
                    <span className="health-check__item-full-path numeric">{r.path}</span>
                    {r.error ? <span className="health-check__item-error">{r.error}</span> : null}
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </>
      ) : null}
    </div>
  );
}
