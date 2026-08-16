import "./Placeholder.css";

interface PlaceholderProps {
  screen: string;
  phase: string;
}

/** Tela de espaço reservado para rotas ainda não implementadas (Fases B/C/D/E). */
export function Placeholder({ screen, phase }: PlaceholderProps) {
  return (
    <div className="placeholder">
      <div className="placeholder__inner">
        <span className="placeholder__phase">Em construção — {phase}</span>
        <span className="placeholder__text">A tela &ldquo;{screen}&rdquo; ainda não foi implementada.</span>
      </div>
    </div>
  );
}
