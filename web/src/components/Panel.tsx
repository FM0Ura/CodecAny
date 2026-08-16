import type { PropsWithChildren, ReactNode } from "react";
import "./Panel.css";

interface PanelProps extends PropsWithChildren {
  title?: ReactNode;
  depth?: "surface" | "ground";
  className?: string;
}

export function Panel({ title, depth = "surface", className, children }: PanelProps) {
  return (
    <section
      className={`panel${depth === "ground" ? " panel--ground" : ""}${className ? ` ${className}` : ""}`}
    >
      {title ? <h2 className="panel__title">{title}</h2> : null}
      {children}
    </section>
  );
}
