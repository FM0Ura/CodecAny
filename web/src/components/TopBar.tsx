import type { PropsWithChildren, ReactNode } from "react";
import "./TopBar.css";

interface TopBarProps extends PropsWithChildren {
  title: string;
  ticker?: ReactNode;
}

export function TopBar({ title, ticker, children }: TopBarProps) {
  return (
    <header className="top-bar">
      <h1 className="top-bar__title">{title}</h1>
      {ticker ?? children}
    </header>
  );
}

interface LiveDotProps {
  status: "connecting" | "open" | "error";
  label?: string;
}

/** Indicador de conexão SSE ao vivo, para uso na área de ticker do TopBar. */
export function LiveDot({ status, label }: LiveDotProps) {
  const text = label ?? (status === "open" ? "AO VIVO" : status === "error" ? "DESCONECTADO" : "CONECTANDO");
  return (
    <span className="top-bar__ticker">
      <span className={`top-bar__dot top-bar__dot--${status}`} aria-hidden="true" />
      {text}
    </span>
  );
}
