import type { JobStatus } from "../api/types";
import "./Chip.css";

export type ChipVariant = "queued" | "progress" | "awaiting" | "completed" | "failed" | "reverted" | "ignored";

const VARIANT_LABEL: Record<ChipVariant, string> = {
  queued: "Na fila",
  progress: "Em progresso",
  awaiting: "Aguardando aprovação",
  completed: "Concluído",
  failed: "Falhou",
  reverted: "Revertido",
  ignored: "Ignorado",
};

interface ChipProps {
  variant: ChipVariant;
  /** Sobrescreve o texto padrão do variant (ex.: status bruto do backend). */
  label?: string;
  className?: string;
}

export function Chip({ variant, label, className }: ChipProps) {
  return (
    <span className={`chip chip--${variant}${className ? ` ${className}` : ""}`}>
      {label ?? VARIANT_LABEL[variant]}
    </span>
  );
}

/** Mapeia o status bruto do Job (backend) para a variante visual do Chip. */
export function statusToVariant(status: JobStatus): ChipVariant {
  switch (status) {
    case "DISCOVERED":
    case "QUEUED":
      return "queued";
    case "IN_PROGRESS":
    case "TESTING":
    case "FINALIZING":
      return "progress";
    case "AWAITING_APPROVAL":
      return "awaiting";
    case "COMPLETED":
      return "completed";
    case "FAILED":
      return "failed";
    case "ROLLED_BACK":
      return "reverted";
    case "IGNORED":
      return "ignored";
    default:
      return "queued";
  }
}

const STATUS_LABEL: Record<JobStatus, string> = {
  DISCOVERED: "Descoberto",
  QUEUED: "Na fila",
  IN_PROGRESS: "Em progresso",
  TESTING: "Testando",
  FINALIZING: "Finalizando",
  COMPLETED: "Concluído",
  FAILED: "Falhou",
  ROLLED_BACK: "Revertido",
  AWAITING_APPROVAL: "Aguardando aprovação",
  IGNORED: "Ignorado",
};

/** Chip pronto a partir de um JobStatus bruto do backend. */
export function StatusChip({ status }: { status: JobStatus }) {
  return <Chip variant={statusToVariant(status)} label={STATUS_LABEL[status]} />;
}
