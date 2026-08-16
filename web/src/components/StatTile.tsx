import type { ReactNode } from "react";
import "./StatTile.css";

interface StatTileProps {
  label: string;
  value: ReactNode;
  context?: ReactNode;
  accent?: boolean;
}

export function StatTile({ label, value, context, accent }: StatTileProps) {
  return (
    <div className="stat-tile">
      <span className="stat-tile__label">{label}</span>
      <span className={`stat-tile__value${accent ? " stat-tile__value--accent" : ""}`}>{value}</span>
      {context ? <span className="stat-tile__context">{context}</span> : null}
    </div>
  );
}
