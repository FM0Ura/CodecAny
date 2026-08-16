import type { DashboardSummary, Job, JobEvent, ServerStatus } from "./types";

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

async function getJSON<T>(path: string): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, { headers: { Accept: "application/json" } });
  } catch {
    throw new ApiError("Não foi possível contatar o servidor CodecAny.");
  }
  if (!res.ok) {
    throw new ApiError(`Requisição falhou (${res.status} ${res.statusText})`, res.status);
  }
  try {
    return (await res.json()) as T;
  } catch {
    throw new ApiError("Resposta do servidor não é um JSON válido.");
  }
}

/** POST genérico sem corpo de requisição (só decodifica a resposta JSON). */
async function postJSON<T>(path: string): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, { method: "POST", headers: { Accept: "application/json" } });
  } catch {
    throw new ApiError("Não foi possível contatar o servidor CodecAny.");
  }
  if (!res.ok) {
    let msg = `Requisição falhou (${res.status} ${res.statusText})`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      // corpo não é JSON — mantém a mensagem padrão.
    }
    throw new ApiError(msg, res.status);
  }
  try {
    return (await res.json()) as T;
  } catch {
    throw new ApiError("Resposta do servidor não é um JSON válido.");
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
