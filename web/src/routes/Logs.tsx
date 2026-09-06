import { useEffect, useMemo, useRef, useState } from "react";
import { getServerLogs, subscribeToEvents } from "../api/client";
import type { JobEvent, LogEntry } from "../api/types";
import { Panel } from "../components/Panel";
import "./Logs.css";

interface UnifiedLog {
  id: string;
  time: string;
  level: "INFO" | "WARN" | "ERROR" | "DEBUG" | "EVENT";
  category: "SERVER" | "ENGINE" | "TRANSCODE" | "EVENT";
  message: string;
  jobId?: string;
  path?: string;
  raw?: string;
}

export function Logs() {
  const [logs, setLogs] = useState<UnifiedLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [sseStatus, setSseStatus] = useState<"connecting" | "open" | "error">("connecting");
  const [levelFilter, setLevelFilter] = useState<string>("ALL");
  const [searchQuery, setSearchQuery] = useState("");
  const [autoScroll, setAutoScroll] = useState(true);
  const [copied, setCopied] = useState(false);

  const terminalRef = useRef<HTMLDivElement>(null);
  const nextIdRef = useRef(0);

  // Carrega histórico de logs do servidor
  const loadInitialLogs = async () => {
    setLoading(true);
    try {
      const resp = await getServerLogs(250);
      const mapped: UnifiedLog[] = resp.lines.map((l: LogEntry) => {
        let lvl: UnifiedLog["level"] = "INFO";
        const upper = l.level.toUpperCase();
        if (upper.includes("WARN")) lvl = "WARN";
        else if (upper.includes("ERR")) lvl = "ERROR";
        else if (upper.includes("DEBUG")) lvl = "DEBUG";

        return {
          id: `srv-${++nextIdRef.current}`,
          time: l.time,
          level: lvl,
          category: "SERVER",
          message: l.msg || l.raw,
          jobId: l.job_id,
          path: l.path,
          raw: l.raw,
        };
      });
      // Mais recentes no topo (ordem cronológica decrescente)
      setLogs(mapped.reverse());
    } catch {
      // Se falhar a leitura inicial, inicializa com aviso de conexão
      setLogs([
        {
          id: `srv-${++nextIdRef.current}`,
          time: new Date().toISOString(),
          level: "WARN",
          category: "SERVER",
          message: "Logs anteriores não puderam ser recuperados do disco. Aguardando novos eventos ao vivo...",
        },
      ]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadInitialLogs();
  }, []);

  // Conecta ao fluxo de eventos em tempo real via SSE
  useEffect(() => {
    const unsubscribe = subscribeToEvents(
      (ev: JobEvent) => {
        let lvl: UnifiedLog["level"] = "EVENT";
        let msg = "";

        switch (ev.kind) {
          case "OnJobStart":
            msg = `[INÍCIO] Conversão iniciada via driver "${ev.driver_used}" • ${ev.file_path || ev.job_id}`;
            lvl = "INFO";
            break;
          case "OnJobProgress": {
            const pct = typeof ev.progress === "number" ? (ev.progress * 100).toFixed(1) : "0.0";
            msg = `[PROGRESSO ${pct}%] Transcodificando ${ev.file_path || ev.job_id}`;
            lvl = "INFO";
            break;
          }
          case "OnJobComplete": {
            const savedPct = ev.metrics ? ev.metrics.compression_ratio_pct.toFixed(1) : "0.0";
            msg = `[CONCLUÍDO] Sucesso! Economia de ${savedPct}% • ${ev.file_path || ev.job_id}`;
            lvl = "INFO";
            break;
          }
          case "OnJobError":
            msg = `[FALHA] ${ev.error || "Erro durante processamento"} • ${ev.file_path || ev.job_id}`;
            lvl = "ERROR";
            break;
          case "OnJobAwaitingApproval":
            msg = `[AGUARDANDO APROVAÇÃO] Conversão pronta para inspeção no staging • ${ev.file_path || ev.job_id}`;
            lvl = "WARN";
            break;
          case "OnJobRequeued":
            msg = `[REENFILEIRADO] Job recolocado na fila com prioridade máxima • ${ev.file_path || ev.job_id}`;
            lvl = "INFO";
            break;
          default:
            msg = `[${ev.kind}] Evento recebido para ${ev.file_path || ev.job_id}`;
        }

        const newLog: UnifiedLog = {
          id: `ev-${++nextIdRef.current}`,
          time: new Date().toISOString(),
          level: lvl,
          category: "TRANSCODE",
          message: msg,
          jobId: ev.job_id,
          path: ev.file_path,
        };

        setLogs((prev) => {
          // Mais recentes no topo ([newLog, ...prev]), limite de 1000 logs
          const updated = [newLog, ...prev];
          if (updated.length > 1000) {
            return updated.slice(0, 1000);
          }
          return updated;
        });
      },
      (status) => {
        setSseStatus(status === "open" ? "open" : "error");
      },
    );

    return () => unsubscribe();
  }, []);

  // Auto-scroll do terminal para o topo (onde os novos logs chegam)
  useEffect(() => {
    if (autoScroll && terminalRef.current) {
      terminalRef.current.scrollTop = 0;
    }
  }, [logs, autoScroll]);

  // Filtros de nível e busca
  const filteredLogs = useMemo(() => {
    return logs.filter((log) => {
      if (levelFilter !== "ALL" && log.level !== levelFilter) {
        return false;
      }
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        const text = `${log.time} ${log.level} ${log.category} ${log.message} ${log.path ?? ""} ${log.jobId ?? ""}`.toLowerCase();
        return text.includes(q);
      }
      return true;
    });
  }, [logs, levelFilter, searchQuery]);

  const copyAll = () => {
    const text = filteredLogs
      .map((l) => `[${formatTime(l.time)}] [${l.level}] [${l.category}] ${l.message}`)
      .join("\n");
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const downloadLogFile = () => {
    const text = filteredLogs
      .map((l) => `[${l.time}] [${l.level}] [${l.category}] ${l.message}`)
      .join("\n");
    const blob = new Blob([text], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `codecany-logs-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-")}.log`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="logs-view">
      <div className="logs-view__toolbar">
        <div className="logs-view__title-group">
          <h1 className="font-display logs-view__title">Console de Logs</h1>
          <div className={`logs-view__sse-badge logs-view__sse-badge--${sseStatus}`}>
            <span className="logs-view__sse-dot" />
            <span>{sseStatus === "open" ? "LIVE STREAM" : sseStatus === "connecting" ? "CONECTANDO" : "OFFLINE"}</span>
          </div>
        </div>

        <div className="logs-view__actions">
          <button type="button" onClick={loadInitialLogs} disabled={loading} title="Recarregar logs do servidor">
            {loading ? "Carregando…" : "Recarregar"}
          </button>
          <button type="button" onClick={copyAll} disabled={filteredLogs.length === 0} title="Copiar linhas filtradas">
            {copied ? "Copiado!" : "Copiar"}
          </button>
          <button type="button" onClick={downloadLogFile} disabled={filteredLogs.length === 0} title="Baixar como arquivo de log">
            Baixar .log
          </button>
          <button type="button" onClick={() => setLogs([])} title="Limpar console atual">
            Limpar
          </button>
        </div>
      </div>

      <Panel>
        <div className="logs-view__controls">
          <div className="logs-view__filter-levels">
            {(["ALL", "INFO", "WARN", "ERROR", "EVENT"] as const).map((lvl) => (
              <button
                key={lvl}
                type="button"
                className={`logs-view__level-btn logs-view__level-btn--${lvl.toLowerCase()} ${
                  levelFilter === lvl ? "is-active" : ""
                }`}
                onClick={() => setLevelFilter(lvl)}
              >
                {lvl === "ALL" ? "TODOS" : lvl}
              </button>
            ))}
          </div>

          <div className="logs-view__search-wrap">
            <input
              type="search"
              className="numeric logs-view__search"
              placeholder="Filtrar por mensagem, arquivo, job id..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
            {searchQuery && (
              <button type="button" className="logs-view__search-clear" onClick={() => setSearchQuery("")}>
                ✕
              </button>
            )}
          </div>

          <label className="logs-view__autoscroll-label">
            <input
              type="checkbox"
              checked={autoScroll}
              onChange={(e) => setAutoScroll(e.target.checked)}
            />
            Auto-scroll
          </label>
        </div>

        {/* Terminal Container */}
        <div className="logs-terminal">
          <div className="logs-terminal__header">
            <div className="logs-terminal__dots">
              <span className="logs-terminal__dot logs-terminal__dot--red" />
              <span className="logs-terminal__dot logs-terminal__dot--yellow" />
              <span className="logs-terminal__dot logs-terminal__dot--green" />
            </div>
            <div className="logs-terminal__title numeric">
              codecany-daemon // tty01 — {filteredLogs.length} {filteredLogs.length === 1 ? "linha" : "linhas"}
            </div>
            <div className="logs-terminal__spacer" />
          </div>

          <div className="logs-terminal__body numeric" ref={terminalRef}>
            {filteredLogs.length === 0 ? (
              <div className="logs-terminal__empty">
                {loading ? "Carregando registros..." : "Nenhum log corresponde aos filtros atuais."}
              </div>
            ) : (
              filteredLogs.map((log) => (
                <div key={log.id} className={`logs-terminal__line logs-terminal__line--${log.level.toLowerCase()}`}>
                  <span className="logs-terminal__time">{formatTime(log.time)}</span>
                  <span className={`logs-terminal__badge logs-terminal__badge--${log.level.toLowerCase()}`}>
                    {log.level.padEnd(5, " ")}
                  </span>
                  <span className="logs-terminal__cat">[{log.category}]</span>
                  <span className="logs-terminal__msg">{log.message}</span>
                </div>
              ))
            )}
          </div>
        </div>
      </Panel>
    </div>
  );
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toTimeString().split(" ")[0] + "." + String(d.getMilliseconds()).padStart(3, "0");
  } catch {
    return iso;
  }
}
