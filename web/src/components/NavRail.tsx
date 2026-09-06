import type { ReactElement } from "react";
import { NavLink } from "react-router-dom";
import {
  IconApproval,
  IconDashboard,
  IconDirectories,
  IconHealth,
  IconHistory,
  IconLogs,
  IconQueue,
  IconRules,
} from "./icons";
import "./NavRail.css";

interface NavItem {
  to: string;
  label: string;
  icon: (props: { width?: number; height?: number }) => ReactElement;
  badgeKey?: "queue" | "approval";
}

const ITEMS: NavItem[] = [
  { to: "/", label: "Dashboard", icon: IconDashboard },
  { to: "/fila", label: "Fila", icon: IconQueue, badgeKey: "queue" },
  { to: "/aprovacao", label: "Aprovação", icon: IconApproval, badgeKey: "approval" },
  { to: "/diretorios", label: "Diretórios", icon: IconDirectories },
  { to: "/regras", label: "Regras", icon: IconRules },
  { to: "/health-check", label: "Health Check", icon: IconHealth },
  { to: "/historico", label: "Histórico", icon: IconHistory },
  { to: "/logs", label: "Logs", icon: IconLogs },
];

interface NavRailProps {
  queueCount?: number;
  approvalCount?: number;
}

export function NavRail({ queueCount, approvalCount }: NavRailProps) {
  const badges: Record<string, number | undefined> = {
    queue: queueCount,
    approval: approvalCount,
  };

  return (
    <nav className="nav-rail" aria-label="Navegação principal">
      <div className="nav-rail__brand">
        <span className="nav-rail__led" aria-hidden="true" />
        <span className="nav-rail__brand-name">CODECANY</span>
      </div>
      <ul className="nav-rail__list">
        {ITEMS.map((item) => {
          const Icon = item.icon;
          const badge = item.badgeKey ? badges[item.badgeKey] : undefined;
          return (
            <li key={item.to}>
              <NavLink
                to={item.to}
                end={item.to === "/"}
                className={({ isActive }) => `nav-rail__link${isActive ? " is-active" : ""}`}
              >
                <Icon width={18} height={18} />
                <span className="nav-rail__label">{item.label}</span>
                {typeof badge === "number" && badge > 0 ? (
                  <span className="nav-rail__badge numeric">{badge}</span>
                ) : null}
              </NavLink>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
