import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import type { PropsWithChildren } from "react";
import { ApiError, getDashboardSummary } from "../api/client";
import type { DashboardSummary } from "../api/types";

const POLL_INTERVAL_MS = 15_000;

interface DashboardSummaryState {
  summary: DashboardSummary | null;
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

const DashboardSummaryContext = createContext<DashboardSummaryState | null>(null);

export function DashboardSummaryProvider({ children }: PropsWithChildren) {
  const [summary, setSummary] = useState<DashboardSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const inFlight = useRef(false);

  const load = useCallback(async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    try {
      const data = await getDashboardSummary();
      setSummary(data);
      setError(null);
    } catch (err) {
      const message = err instanceof ApiError ? err.message : "Falha ao carregar o resumo do painel.";
      setError(message);
    } finally {
      setLoading(false);
      inFlight.current = false;
    }
  }, []);

  useEffect(() => {
    load();
    const timer = window.setInterval(load, POLL_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [load]);

  return (
    <DashboardSummaryContext.Provider value={{ summary, loading, error, refresh: load }}>
      {children}
    </DashboardSummaryContext.Provider>
  );
}

export function useDashboardSummary(): DashboardSummaryState {
  const ctx = useContext(DashboardSummaryContext);
  if (!ctx) {
    throw new Error("useDashboardSummary deve ser usado dentro de DashboardSummaryProvider");
  }
  return ctx;
}
