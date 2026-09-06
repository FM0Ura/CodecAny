import type {
  DashboardSummary,
  FsBrowseResult,
  HealthCheckResult,
  Job,
  JobEvent,
  RuleFile,
  RuleTestRequest,
  RuleTestResult,
  ServerStatus,
  WatchedDir,
} from "./types";

/**
 * Erro lançado quando uma resposta HTTP não é ok (status fora de 2xx) ou o
 * corpo não é o JSON esperado. Mantém o status para a UI poder diferenciar
 * "servidor fora do ar" de "erro de aplicação".
 */
export class ApiError extends Error {
  status?: number;

  constructor(message: string, status?: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

/**
 * requestJSON centraliza fetch + tratamento de erro para toda a API: em
 * respostas não-ok, tenta ler `{"error": "..."}` (shape de writeJSONError no
 * backend, ver cmd/server/router.go) para propagar uma mensagem legível na
 * UI (ex.: erro de validação de PUT /api/rules) em vez de só o status HTTP.
 */
async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      ...init,
      headers: {
        Accept: "application/json",
        ...(init?.body ? { "Content-Type": "application/json" } : {}),
        ...init?.headers,
      },
    });
  } catch {
    throw new ApiError("Não foi possível contatar o servidor CodecAny.");
  }
  if (!res.ok) {
    let message = `Requisição falhou (${res.status} ${res.statusText})`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body?.error) message = body.error;
    } catch {
      // corpo de erro não é JSON — mantém a mensagem genérica de status.
    }
    throw new ApiError(message, res.status);
  }
  try {
    return (await res.json()) as T;
  } catch {
    throw new ApiError("Resposta do servidor não é um JSON válido.");
  }
}

function getJSON<T>(path: string): Promise<T> {
  return requestJSON<T>(path);
}

function putJSON<T>(path: string, body: unknown): Promise<T> {
  return requestJSON<T>(path, { method: "PUT", body: JSON.stringify(body) });
}

/** POST com corpo JSON opcional, decodificando a resposta como T. */
function postJSON<T>(path: string, body?: unknown): Promise<T> {
  return requestJSON<T>(path, {
    method: "POST",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

/** DELETE decodificando a resposta como T (o backend sempre responde JSON, ver writeJSON). */
function del<T>(path: string): Promise<T> {
  return requestJSON<T>(path, { method: "DELETE" });
}

export function getStatus(): Promise<ServerStatus> {
  return getJSON<ServerStatus>("/api/status");
}

export function getDashboardSummary(): Promise<DashboardSummary> {
  return getJSON<DashboardSummary>("/api/dashboard/summary");
}

export interface JobFilter {
  status?: string;
  since?: string;
  until?: string;
  dir?: string;
}

export function getJobs(filter: JobFilter = {}): Promise<Job[]> {
  const params = new URLSearchParams();
  if (filter.status) params.set("status", filter.status);
  if (filter.since) params.set("since", filter.since);
  if (filter.until) params.set("until", filter.until);
  if (filter.dir) params.set("dir", filter.dir);
  const qs = params.toString();
  return getJSON<Job[]>(`/api/jobs${qs ? `?${qs}` : ""}`);
}

export function getJob(id: string): Promise<Job> {
  return getJSON<Job>(`/api/jobs/${encodeURIComponent(id)}`);
}

/** POST /api/jobs/{id}/requeue — reenfileira um job FAILED/ROLLED_BACK com prioridade máxima; 404 se não existir/não estiver nesses status. */
export function requeueJob(id: string): Promise<OkResponse> {
  return postJSON<OkResponse>(`/api/jobs/${encodeURIComponent(id)}/requeue`);
}

export interface HealthCheckRequest {
  dirs?: string[];
  files?: string[];
}

/**
 * Dispara POST /api/health-check (Fase E). Síncrono no servidor — pode
 * demorar bastante para bibliotecas grandes; a UI deve mostrar um indicador
 * de carregamento enquanto aguarda. Corpo vazio ({}) usa o fallback de
 * diretórios monitorados configurados no boot do servidor (ver
 * cmd/server/healthcheck.go).
 */
export function runHealthCheck(req: HealthCheckRequest = {}): Promise<HealthCheckResult[]> {
  return postJSON<HealthCheckResult[]>("/api/health-check", req);
}

const EVENT_KINDS: JobEvent["kind"][] = [
  "OnJobStart",
  "OnJobProgress",
  "OnJobComplete",
  "OnJobError",
  "OnJobAwaitingApproval",
  "OnJobRequeued",
];

/**
 * Abre a conexão SSE de /api/events e invoca `onEvent` para cada frame
 * recebido, decodificado como JobEvent. Retorna uma função de cleanup que
 * fecha a conexão (chame no cleanup de um useEffect).
 */
export function subscribeToEvents(
  onEvent: (event: JobEvent) => void,
  onStatusChange?: (status: "open" | "error") => void,
): () => void {
  const source = new EventSource("/api/events");

  source.onopen = () => onStatusChange?.("open");
  source.onerror = () => onStatusChange?.("error");

  const handler = (kind: JobEvent["kind"]) => (raw: MessageEvent<string>) => {
    try {
      const parsed = JSON.parse(raw.data) as Omit<JobEvent, "kind">;
      onEvent({ ...parsed, kind });
    } catch {
      // Frame malformado — ignora silenciosamente, não derruba a conexão.
    }
  };

  for (const kind of EVENT_KINDS) {
    source.addEventListener(kind, handler(kind));
  }

  return () => source.close();
}

// --- Staging/Aprovação (Fase B) ------------------------------------------

export interface OkResponse {
  ok: boolean;
}

export interface ApproveAllResponse {
  approved: number;
  errors: string[];
}

export interface RejectAllResponse {
  rejected: number;
  errors: string[];
}

/** GET /api/staging — jobs em StatusAwaitingApproval (Fase B). */
export function getStaging(): Promise<Job[]> {
  return getJSON<Job[]>("/api/staging");
}

/** POST /api/staging/{id}/approve — comita o output em staging, 404 se o job não existir/não estiver em staging. */
export function approveJob(id: string): Promise<OkResponse> {
  return postJSON<OkResponse>(`/api/staging/${encodeURIComponent(id)}/approve`);
}

/** POST /api/staging/{id}/reject — descarta o output em staging, preservando o original. */
export function rejectJob(id: string): Promise<OkResponse> {
  return postJSON<OkResponse>(`/api/staging/${encodeURIComponent(id)}/reject`);
}

/** POST /api/staging/approve-all — sucesso parcial ainda responde 200 com a lista de erros. */
export function approveAll(): Promise<ApproveAllResponse> {
  return postJSON<ApproveAllResponse>("/api/staging/approve-all");
}

/** POST /api/staging/reject-all — mesma semântica de sucesso parcial de approveAll. */
export function rejectAll(): Promise<RejectAllResponse> {
  return postJSON<RejectAllResponse>("/api/staging/reject-all");
}

// --- Jobs / Fila (Fase A/G) ---------------------------------------------


/** POST /api/jobs/{id}/cancel — aborta um job em andamento ou cancela um job na fila. */
export function cancelJob(id: string): Promise<OkResponse> {
  return postJSON<OkResponse>(`/api/jobs/${encodeURIComponent(id)}/cancel`);
}

/** POST /api/queue/cancel-all — cancela todos os jobs em andamento e pendentes na fila. */
export function cancelAllJobs(): Promise<{ ok: boolean; count: number }> {
  return postJSON<{ ok: boolean; count: number }>("/api/queue/cancel-all");
}

/** POST /api/queue/pause — pausa o processamento de novos jobs da fila. */
export function pauseQueue(): Promise<{ ok: boolean; paused: boolean }> {
  return postJSON<{ ok: boolean; paused: boolean }>("/api/queue/pause");
}

/** POST /api/queue/resume — retoma o processamento de novos jobs da fila. */
export function resumeQueue(): Promise<{ ok: boolean; paused: boolean }> {
  return postJSON<{ ok: boolean; paused: boolean }>("/api/queue/resume");
}

/** GET /api/queue/status — obtém o status atual da fila (se está pausada). */
export function getQueueStatus(): Promise<{ paused: boolean }> {
  return getJSON<{ paused: boolean }>("/api/queue/status");
}

// --- Diretórios Monitorados (Fase C) -------------------------------------

/** Lista os diretórios monitorados persistidos acompanhados de estatísticas. */
export function getDirs(): Promise<WatchedDir[]> {
  return getJSON<WatchedDir[]>("/api/dirs");
}

/**
 * Adiciona um diretório à lista de monitorados. O backend valida que path
 * existe e é diretório (400 caso contrário, ou se já estiver monitorado).
 */
export function addDir(path: string): Promise<{ path: string }> {
  return postJSON<{ path: string }>("/api/dirs", { path });
}

/** Remove um diretório da lista de monitorados. */
export function removeDir(path: string): Promise<{ status: string }> {
  return del<{ status: string }>(`/api/dirs?path=${encodeURIComponent(path)}`);
}

/** Redescobre arquivos já presentes nos diretórios monitorados (ou em um específico). */
export function rescanDirs(path?: string): Promise<void> {
  const qs = path ? `?path=${encodeURIComponent(path)}` : "";
  return postJSON(`/api/dirs/rescan${qs}`, path ? { path } : undefined);
}

/**
 * Navega o filesystem do servidor a partir de path (só diretórios,
 * ordenados). path="" retorna a raiz. Usado pelo modal de "Adicionar
 * diretório" — navegador em vez de campo de texto puro.
 */
export function browseFs(path: string): Promise<FsBrowseResult> {
  const qs = path ? `?path=${encodeURIComponent(path)}` : "";
  return getJSON<FsBrowseResult>(`/api/fs/browse${qs}`);
}

// --- Regras/Config (Fase D) -----------------------------------------------

/** GET /api/rules — o RuleFile atualmente em uso pelo servidor. */
export function getRules(): Promise<RuleFile> {
  return getJSON<RuleFile>("/api/rules");
}

/**
 * PUT /api/rules — envia o RuleFile completo (regras na ordem final
 * desejada — reordenar é reenviar o array inteiro). Em caso de validação
 * inválida, o backend responde 400 com `{error}` (propagado como
 * ApiError.message por requestJSON) e NÃO altera o arquivo em disco.
 * Retorna o RuleFile persistido (refletindo o hot-reload já aplicado).
 */
export function putRules(file: RuleFile): Promise<RuleFile> {
  return putJSON<RuleFile>("/api/rules", file);
}

/** POST /api/rules/test — avalia as regras atuais contra `path` ou `media_info`. */
export function testRule(req: RuleTestRequest): Promise<RuleTestResult> {
  return postJSON<RuleTestResult>("/api/rules/test", req);
}

// --- Logs do Servidor ----------------------------------------------------

/** GET /api/logs?lines=200 — últimas linhas de log do servidor. */
export function getServerLogs(lines: number = 200): Promise<import("./types").LogsResponse> {
  return getJSON<import("./types").LogsResponse>(`/api/logs?lines=${lines}`);
}
