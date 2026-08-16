import type {
  DashboardSummary,
  Job,
  JobEvent,
  RuleFile,
  RuleTestRequest,
  RuleTestResult,
  ServerStatus,
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

function postJSON<T>(path: string, body: unknown): Promise<T> {
  return requestJSON<T>(path, { method: "POST", body: JSON.stringify(body) });
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
