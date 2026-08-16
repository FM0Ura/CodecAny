import { Outlet, useLocation } from "react-router-dom";
import { NavRail } from "./NavRail";
import { TopBar } from "./TopBar";
import { useDashboardSummary } from "../context/DashboardSummaryContext";
import "./AppLayout.css";

const TITLES: Record<string, string> = {
  "/": "Dashboard",
  "/fila": "Fila",
  "/aprovacao": "Aguardando Aprovação",
  "/diretorios": "Diretórios Monitorados",
  "/regras": "Regras",
  "/health-check": "Health Check",
  "/historico": "Histórico",
};

const ACTIVE_STATUSES = ["QUEUED", "IN_PROGRESS", "TESTING", "FINALIZING"] as const;

export function AppLayout() {
  const { pathname } = useLocation();
  const { summary } = useDashboardSummary();
  const title = TITLES[pathname] ?? "CodecAny";

  const queueCount = summary
    ? ACTIVE_STATUSES.reduce((sum, status) => sum + (summary.status_counts[status] ?? 0), 0)
    : undefined;
  const approvalCount = summary?.status_counts.AWAITING_APPROVAL;

  return (
    <>
      <NavRail queueCount={queueCount} approvalCount={approvalCount} />
      <div className="app-layout__main">
        <TopBar title={title} />
        <main className="app-layout__content">
          <Outlet />
        </main>
      </div>
    </>
  );
}
