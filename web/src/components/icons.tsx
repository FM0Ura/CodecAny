import type { SVGProps } from "react";

// Ícones lineares simples (sem emoji), 20x20, stroke=currentColor.
// Usados no NavRail — um por rota.

const base: SVGProps<SVGSVGElement> = {
  width: 18,
  height: 18,
  viewBox: "0 0 20 20",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.5,
  strokeLinecap: "round",
  strokeLinejoin: "round",
};

export function IconDashboard(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props}>
      <rect x="2.5" y="2.5" width="6.5" height="6.5" rx="1" />
      <rect x="11" y="2.5" width="6.5" height="4" rx="1" />
      <rect x="11" y="8.5" width="6.5" height="9" rx="1" />
      <rect x="2.5" y="11" width="6.5" height="6.5" rx="1" />
    </svg>
  );
}

export function IconQueue(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props}>
      <rect x="2.5" y="3.5" width="15" height="3.6" rx="1" />
      <rect x="2.5" y="8.2" width="15" height="3.6" rx="1" />
      <rect x="2.5" y="12.9" width="15" height="3.6" rx="1" />
    </svg>
  );
}

export function IconApproval(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props}>
      <circle cx="10" cy="10" r="7.5" />
      <path d="M6.7 10.2l2.1 2.1 4.5-4.6" />
    </svg>
  );
}

export function IconDirectories(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props}>
      <path d="M2.5 5.2c0-.7.6-1.2 1.2-1.2h3.4l1.6 1.8h7.1c.7 0 1.2.6 1.2 1.2v8.3c0 .7-.6 1.2-1.2 1.2H3.7c-.7 0-1.2-.6-1.2-1.2V5.2z" />
    </svg>
  );
}

export function IconRules(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props}>
      <line x1="3" y1="5.5" x2="17" y2="5.5" />
      <line x1="3" y1="10" x2="17" y2="10" />
      <line x1="3" y1="14.5" x2="17" y2="14.5" />
      <circle cx="7" cy="5.5" r="1.6" fill="currentColor" stroke="none" />
      <circle cx="13.5" cy="10" r="1.6" fill="currentColor" stroke="none" />
      <circle cx="9" cy="14.5" r="1.6" fill="currentColor" stroke="none" />
    </svg>
  );
}

export function IconHealth(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props}>
      <path d="M2.5 10.5h3.4l1.6-4 2.6 7.4 1.7-4.6h5.7" />
    </svg>
  );
}

export function IconHistory(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props}>
      <circle cx="10" cy="10.5" r="7" />
      <path d="M10 6.3v4.4l3 1.8" />
      <path d="M4.2 4.2L3 3v3.4" />
    </svg>
  );
}

// Ícones auxiliares da tela Diretórios Monitorados (Fase C) — mesmo estilo
// linear/sem emoji dos ícones de navegação acima.

export function IconTrash(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props}>
      <path d="M4 5.5h12" />
      <path d="M8 5.5V4c0-.6.4-1 1-1h2c.6 0 1 .4 1 1v1.5" />
      <path d="M5.3 5.5l.6 10c.1.9.8 1.5 1.6 1.5h4.9c.8 0 1.5-.6 1.6-1.5l.6-10" />
      <path d="M8.4 8.5v6" />
      <path d="M11.6 8.5v6" />
    </svg>
  );
}

export function IconClose(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props}>
      <path d="M4.5 4.5l11 11" />
      <path d="M15.5 4.5l-11 11" />
    </svg>
  );
}

export function IconFolder(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props}>
      <path d="M2.5 5.2c0-.7.6-1.2 1.2-1.2h3.4l1.6 1.8h7.1c.7 0 1.2.6 1.2 1.2v8.3c0 .7-.6 1.2-1.2 1.2H3.7c-.7 0-1.2-.6-1.2-1.2V5.2z" />
    </svg>
  );
}

export function IconArrowLeft(props: SVGProps<SVGSVGElement>) {
  return (
    <svg {...base} {...props}>
      <path d="M12.5 4.5l-6 5.5 6 5.5" />
    </svg>
  );
}
