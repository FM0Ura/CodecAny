import type { DashboardSummary, FsBrowseResult, Job, JobEvent, ServerStatus } from "./types";

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
 * Extrai a mensagem de erro do corpo `{"error": "..."}` que
 * writeJSONError (cmd/server/router.go) escreve em toda resposta não-2xx.
 * Cai para uma mensagem genérica se o corpo não for esse shape.
 */
async function extractErrorMessage(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string };
    if (body?.error) return body.error;
  } catch {
    // corpo não é JSON (ou já foi consumido) — mensagem genérica abaixo.
  }
  return `Requisição falhou (${res.status} ${res.statusText})`;
}

async function getJSON<T>(path: string): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, { headers: { Accept: "application/json" } });
  } catch {
    throw new ApiError("Não foi possível contatar o servidor CodecAny.");
  }
  if (!res.ok) {
    throw new ApiError(await extractErrorMessage(res), res.status);
  }
  try {
    return (await res.json()) as T;
  } catch {
    throw new ApiError("Resposta do servidor não é um JSON válido.");
  }
}

/** POST com corpo JSON opcional, decodificando a resposta como T. */
async function postJSON<T>(path: string, body?: unknown): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new ApiError("Não foi possível contatar o servidor CodecAny.");
  }
  if (!res.ok) {
    throw new ApiError(await extractErrorMessage(res), res.status);
  }
  try {
    return (await res.json()) as T;
  } catch {
    throw new ApiError("Resposta do servidor não é um JSON válido.");
  }
}

/** DELETE sem corpo de resposta relevante. */
async function del(path: string): Promise<void> {
  let res: Response;
  try {
    res = await fetch(path, { method: "DELETE" });
  } catch {
    throw new ApiError("Não foi possível contatar o servidor CodecAny.");
  }
  if (!res.ok) {
    throw new ApiError(await extractErrorMessage(res), res.status);
  }
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
}

export function getJobs(filter: JobFilter = {}): Promise<Job[]> {
  const params = new URLSearchParams();
  if (filter.status) params.set("status", filter.status);
  if (filter.since) params.set("since", filter.since);
  if (filter.until) params.set("until", filter.until);
  const qs = params.toString();
  return getJSON<Job[]>(`/api/jobs${qs ? `?${qs}` : ""}`);
}

export function getJob(id: string): Promise<Job> {
  return getJSON<Job>(`/api/jobs/${encodeURIComponent(id)}`);
}

const EVENT_KINDS: JobEvent["kind"][] = [
  "OnJobStart",
  "OnJobProgress",
  "OnJobComplete",
  "OnJobError",
  "OnJobAwaitingApproval",
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

// --- Diretórios Monitorados (Fase C) -------------------------------------

/** Lista os diretórios monitorados persistidos (watched_dirs). */
export function getDirs(): Promise<string[]> {
  return getJSON<string[]>("/api/dirs");
}

/**
 * Adiciona um diretório à lista de monitorados. O backend valida que path
 * existe e é diretório (400 caso contrário, ou se já estiver monitorado).
 */
export function addDir(path: string): Promise<{ path: string }> {
  return postJSON<{ path: string }>("/api/dirs", { path });
}

/** Remove um diretório da lista de monitorados. */
export function removeDir(path: string): Promise<void> {
  return del(`/api/dirs?path=${encodeURIComponent(path)}`);
}

/** Redescobre arquivos já presentes nos diretórios monitorados. */
export function rescanDirs(): Promise<void> {
  return postJSON("/api/dirs/rescan");
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
