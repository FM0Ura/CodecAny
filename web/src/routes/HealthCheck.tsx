import { useState } from "react";
import { ApiError, runHealthCheck } from "../api/client";
import type { HealthCheckResult } from "../api/types";
import { Panel } from "../components/Panel";
import { StatTile } from "../components/StatTile";
import { basename } from "../lib/format";
import "./HealthCheck.css";

type ScanState = "idle" | "running" | "done" | "error";

/**
 * Tela de Health Check (Fase E, item 11 da proposta). POST /api/health-check
 * é síncrono no v1 — sem streaming/SSE — então a UI só tem três estados:
 * idle (botão + campo opcional de dirs/files), running (indicador de
 * carregamento, sem barra de progresso real já que o servidor não reporta
 * progresso parcial) e done/error (lista de resultados ou mensagem de erro).
 *
 * Campo de dirs/files: um textarea de texto livre (um caminho por linha) —
 * decisão de produto deste v1 para não depender da Fase C (navegador de
 * diretórios via GET /api/fs/browse, ainda não implementada neste branch).
 * Deixar vazio usa o fallback do servidor (diretórios monitorados
 * configurados no boot, ver cmd/server/healthcheck.go).
 */
export function HealthCheck() {
  const [pathsInput, setPathsInput] = useState("");
  const [state, setState] = useState<ScanState>("idle");
  const [results, setResults] = useState<HealthCheckResult[]>([]);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const runScan = async () => {
    setState("running");
    setErrorMessage(null);

    const lines = pathsInput
      .split("\n")
      .map((l) => l.trim())
      .filter(Boolean);
    const dirs = lines.filter((l) => l.endsWith("/"));
    const files = lines.filter((l) => !l.endsWith("/"));

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

        <label className="health-check__label" htmlFor="hc-paths">
          Diretórios/arquivos específicos (opcional — um por linha; diretórios terminam em
          &ldquo;/&rdquo;). Deixe em branco para usar os diretórios monitorados do servidor.
        </label>
        <textarea
          id="hc-paths"
          className="health-check__textarea"
          placeholder={"/media/filmes/\n/media/series/\n/media/extra/arquivo-solto.mkv"}
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
