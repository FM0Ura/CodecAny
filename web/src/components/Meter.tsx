import "./Meter.css";

interface MeterProps {
  label: string;
  /** 0–1 */
  value: number;
  /** Texto formatado exibido à direita do label (ex.: "63%"). */
  valueLabel?: string;
}

/** Medidor de sinal segmentado (leitura contínua) — usa --signal, nunca cor de marca. */
export function Meter({ label, value, valueLabel }: MeterProps) {
  const pct = Math.max(0, Math.min(1, value)) * 100;
  return (
    <div className="meter">
      <div className="meter__head">
        <span className="meter__label">{label}</span>
        <span className="meter__value">{valueLabel ?? `${Math.round(pct)}%`}</span>
      </div>
      <div
        className="meter__track"
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(pct)}
      >
        <div className="meter__fill" style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

interface BlockMeterProps {
  label: string;
  /** Capacidade total (número de blocos desenhados). */
  total: number;
  /** Quantos blocos, da esquerda, aparecem preenchidos. */
  filled: number;
}

/** Medidor de blocos discretos — usado para utilização de hwaccel (limit/in_use). */
export function BlockMeter({ label, total, filled }: BlockMeterProps) {
  const safeTotal = Math.max(total, filled, 1);
  const blocks = Array.from({ length: safeTotal }, (_, i) => i < filled);
  return (
    <div className="block-meter">
      <div className="block-meter__head">
        <span className="block-meter__label">{label}</span>
        <span className="block-meter__value numeric">
          {filled} / {total}
        </span>
      </div>
      <div
        className="block-meter__blocks"
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={total}
        aria-valuenow={filled}
      >
        {blocks.map((isFilled, i) => (
          <div
            key={i}
            className={`block-meter__block${isFilled ? " block-meter__block--filled" : ""}`}
          />
        ))}
      </div>
    </div>
  );
}
