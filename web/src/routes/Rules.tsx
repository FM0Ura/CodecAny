import { useEffect, useState } from "react";
import { ApiError, getRules, putRules, testRule } from "../api/client";
import type {
  ConvertSpec,
  Match,
  Rule,
  RuleFile,
  RuleItem,
  RuleTargetVideo,
  RuleTestResult,
} from "../api/types";
import { Panel } from "../components/Panel";
import "./Rules.css";

/** Item (core.Item) aceita escalar OU lista na API — normaliza pra string[]
 * sempre, tanto pra exibir quanto pra editar. Reenviar como array é seguro:
 * o backend (UnmarshalJSON) aceita escalar ou lista de volta. */
function itemValues(item: RuleItem | undefined | null): string[] {
  if (!item) return [];
  return Array.isArray(item) ? item : [item];
}

function itemFromText(text: string): string[] {
  return text
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

function itemToText(item: RuleItem | undefined | null): string {
  return itemValues(item).join(", ");
}

const DEFAULT_TARGET_VIDEO: RuleTargetVideo = {
  codec: "",
  crf: 0,
  preset: "",
  lossless: null,
  hwaccel: "",
  max_height: 0,
};

let nextTmpId = 0;

/** blankRule cria uma regra nova em branco, pronta para edição no builder. */
function blankRule(): Rule {
  return {
    name: `nova regra ${++nextTmpId}`,
    enabled: null,
    match: {
      container: [],
      video: { codec: [], min_height: 0, max_height: 0, min_bitrate_kbps: 0 },
      audio: { codec: [] },
    },
    action: "convert",
    convert: {
      video: { ...DEFAULT_TARGET_VIDEO },
      audio: null,
      container: "",
    },
    auto_approve: null,
  };
}

/** Botão de três estados (herdar / sim / não) para campos *bool que
 * distinguem "não especificado" (herda default global) de true/false
 * explícito — mesma convenção de core.boolVal (nil ≠ false). */
function TriStateSelect({
  value,
  onChange,
  trueLabel = "Sim",
  falseLabel = "Não",
}: {
  value: boolean | null;
  onChange: (v: boolean | null) => void;
  trueLabel?: string;
  falseLabel?: string;
}) {
  const current = value === null || value === undefined ? "" : value ? "true" : "false";
  return (
    <select
      className="rules__select"
      value={current}
      onChange={(e) => {
        const v = e.target.value;
        onChange(v === "" ? null : v === "true");
      }}
    >
      <option value="">Herdar do default global</option>
      <option value="true">{trueLabel}</option>
      <option value="false">{falseLabel}</option>
    </select>
  );
}

function updateMatch(match: Match, patch: Partial<Match>): Match {
  return { ...match, ...patch };
}

function updateConvert(convert: ConvertSpec, patch: Partial<ConvertSpec>): ConvertSpec {
  return { ...convert, ...patch };
}

interface RuleCardProps {
  rule: Rule;
  index: number;
  total: number;
  expanded: boolean;
  onToggleExpanded: () => void;
  onChange: (next: Rule) => void;
  onRemove: () => void;
  onMove: (delta: -1 | 1) => void;
}

function RuleCard({ rule, index, total, expanded, onToggleExpanded, onChange, onRemove, onMove }: RuleCardProps) {
  const enabled = rule.enabled !== false;
  const video = rule.convert.video;
  const audio = rule.convert.audio;

  return (
    <div className={`rule-card${enabled ? "" : " rule-card--disabled"}`}>
      <div className="rule-card__head">
        <div className="rule-card__order">
          <button type="button" disabled={index === 0} onClick={() => onMove(-1)} title="Mover para cima" aria-label="Mover regra para cima">
            ▲
          </button>
          <button type="button" disabled={index === total - 1} onClick={() => onMove(1)} title="Mover para baixo" aria-label="Mover regra para baixo">
            ▼
          </button>
        </div>
        <span className="rule-card__index numeric">#{index + 1}</span>
        <input
          className="rule-card__name"
          value={rule.name}
          onChange={(e) => onChange({ ...rule, name: e.target.value })}
          placeholder="Nome da regra"
        />
        <label className="rule-card__toggle">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => onChange({ ...rule, enabled: e.target.checked ? null : false })}
          />
          {enabled ? "Habilitada" : "Desabilitada"}
        </label>
        <select
          className="rules__select"
          value={rule.action || "convert"}
          onChange={(e) => onChange({ ...rule, action: e.target.value as Rule["action"] })}
        >
          <option value="convert">convert</option>
          <option value="skip">skip</option>
        </select>
        <button type="button" className="rule-card__expand" onClick={onToggleExpanded}>
          {expanded ? "Recolher" : "Editar"}
        </button>
        <button type="button" className="rule-card__remove" onClick={onRemove} title="Remover regra">
          Remover
        </button>
      </div>

      {expanded ? (
        <div className="rule-card__body">
          <fieldset className="rule-card__fieldset">
            <legend>Match (condições)</legend>
            <div className="rule-card__grid">
              <label>
                Container (lista separada por vírgula)
                <input
                  className="numeric"
                  value={itemToText(rule.match.container)}
                  placeholder="mkv, mp4"
                  onChange={(e) =>
                    onChange({ ...rule, match: updateMatch(rule.match, { container: itemFromText(e.target.value) }) })
                  }
                />
              </label>
              <label>
                video.codec
                <input
                  className="numeric"
                  value={itemToText(rule.match.video.codec)}
                  placeholder="h264, mpeg2video"
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      match: updateMatch(rule.match, {
                        video: { ...rule.match.video, codec: itemFromText(e.target.value) },
                      }),
                    })
                  }
                />
              </label>
              <label>
                video.min_height
                <input
                  type="number"
                  className="numeric"
                  value={rule.match.video.min_height}
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      match: updateMatch(rule.match, {
                        video: { ...rule.match.video, min_height: Number(e.target.value) || 0 },
                      }),
                    })
                  }
                />
              </label>
              <label>
                video.max_height
                <input
                  type="number"
                  className="numeric"
                  value={rule.match.video.max_height}
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      match: updateMatch(rule.match, {
                        video: { ...rule.match.video, max_height: Number(e.target.value) || 0 },
                      }),
                    })
                  }
                />
              </label>
              <label>
                video.min_bitrate_kbps
                <input
                  type="number"
                  className="numeric"
                  value={rule.match.video.min_bitrate_kbps}
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      match: updateMatch(rule.match, {
                        video: { ...rule.match.video, min_bitrate_kbps: Number(e.target.value) || 0 },
                      }),
                    })
                  }
                />
              </label>
              <label>
                audio.codec (não é bloqueante)
                <input
                  className="numeric"
                  value={itemToText(rule.match.audio.codec)}
                  placeholder="aac, ac3"
                  onChange={(e) =>
                    onChange({ ...rule, match: updateMatch(rule.match, { audio: { codec: itemFromText(e.target.value) } }) })
                  }
                />
              </label>
            </div>
          </fieldset>

          <fieldset className="rule-card__fieldset">
            <legend>Convert (alvo)</legend>
            <div className="rule-card__grid">
              <label>
                video.codec
                <input
                  className="numeric"
                  value={video?.codec ?? ""}
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        video: { ...(video ?? DEFAULT_TARGET_VIDEO), codec: e.target.value },
                      }),
                    })
                  }
                />
              </label>
              <label>
                video.crf
                <input
                  type="number"
                  className="numeric"
                  value={video?.crf ?? 0}
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        video: { ...(video ?? DEFAULT_TARGET_VIDEO), crf: Number(e.target.value) || 0 },
                      }),
                    })
                  }
                />
              </label>
              <label>
                video.preset
                <input
                  className="numeric"
                  value={video?.preset ?? ""}
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        video: { ...(video ?? DEFAULT_TARGET_VIDEO), preset: e.target.value },
                      }),
                    })
                  }
                />
              </label>
              <label>
                video.hwaccel
                <input
                  className="numeric"
                  value={video?.hwaccel ?? ""}
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        video: { ...(video ?? DEFAULT_TARGET_VIDEO), hwaccel: e.target.value },
                      }),
                    })
                  }
                />
              </label>
              <label>
                video.max_height
                <input
                  type="number"
                  className="numeric"
                  value={video?.max_height ?? 0}
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        video: { ...(video ?? DEFAULT_TARGET_VIDEO), max_height: Number(e.target.value) || 0 },
                      }),
                    })
                  }
                />
              </label>
              <label>
                video.lossless
                <TriStateSelect
                  value={video?.lossless ?? null}
                  onChange={(v) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        video: { ...(video ?? DEFAULT_TARGET_VIDEO), lossless: v },
                      }),
                    })
                  }
                />
              </label>
              <label>
                audio.codec
                <input
                  className="numeric"
                  value={audio?.codec ?? ""}
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        audio: { codec: e.target.value, bitrate: audio?.bitrate ?? "" },
                      }),
                    })
                  }
                />
              </label>
              <label>
                audio.bitrate
                <input
                  className="numeric"
                  value={audio?.bitrate ?? ""}
                  placeholder="128k"
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        audio: { codec: audio?.codec ?? "", bitrate: e.target.value },
                      }),
                    })
                  }
                />
              </label>
              <label>
                container
                <input
                  className="numeric"
                  value={rule.convert.container}
                  placeholder="auto"
                  onChange={(e) => onChange({ ...rule, convert: updateConvert(rule.convert, { container: e.target.value }) })}
                />
              </label>
              <label>
                auto_approve (override desta regra)
                <TriStateSelect value={rule.auto_approve} onChange={(v) => onChange({ ...rule, auto_approve: v })} />
              </label>
            </div>
          </fieldset>
        </div>
      ) : null}
    </div>
  );
}

function GlobalConfigPanel({ file }: { file: RuleFile }) {
  const g = file.global;
  const limits = Object.entries(g.hwaccel_limits ?? {});
  return (
    <Panel title="Configurações globais (somente leitura)">
      <p className="rules__readonly-note">
        Estes campos vêm de <code className="numeric">global</code> no rules.yaml. Alterá-los exige editar o
        arquivo/flags do processo e <strong>reiniciar o servidor</strong> — o hot-reload deste painel troca só o
        casamento de regras (<code className="numeric">rules</code> + <code className="numeric">global.defaults</code>
        ), nunca estes valores.
      </p>
      <div className="rules__readonly-grid">
        <div>
          <span className="rules__readonly-label">default_driver</span>
          <span className="numeric">{g.default_driver || "—"}</span>
        </div>
        <div>
          <span className="rules__readonly-label">staging_dir</span>
          <span className="numeric">{g.staging_dir || "—"}</span>
        </div>
        <div>
          <span className="rules__readonly-label">space_saving.min_saving_pct</span>
          <span className="numeric">{g.space_saving.min_saving_pct}%</span>
        </div>
        <div>
          <span className="rules__readonly-label">space_saving.fallback_action</span>
          <span className="numeric">{g.space_saving.fallback_action || "—"}</span>
        </div>
        <div>
          <span className="rules__readonly-label">hwaccel_limits</span>
          <span className="numeric">
            {limits.length > 0 ? limits.map(([k, v]) => `${k}:${v}`).join(", ") : "nenhum limite configurado"}
          </span>
        </div>
        <div>
          <span className="rules__readonly-label">notifications.webhook_url</span>
          <span className="numeric">{g.notifications.webhook_url || "—"}</span>
        </div>
      </div>
    </Panel>
  );
}

function RuleTester({ savedFile }: { savedFile: RuleFile | null }) {
  const [path, setPath] = useState("");
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState<RuleTestResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  const run = async () => {
    if (!path.trim()) return;
    setTesting(true);
    setError(null);
    setResult(null);
    try {
      const r = await testRule({ path: path.trim() });
      setResult(r);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Falha ao testar a regra.");
    } finally {
      setTesting(false);
    }
  };

  return (
    <Panel title="Testador de regras">
      {!savedFile ? (
        <span className="dashboard__empty-note">Salve as regras antes de testar (o teste usa as regras já em vigor no servidor).</span>
      ) : null}
      <div className="rules__tester-row">
        <input
          className="numeric rules__tester-input"
          placeholder="/caminho/para/arquivo.mkv"
          value={path}
          onChange={(e) => setPath(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && run()}
        />
        <button type="button" onClick={run} disabled={testing || !path.trim()}>
          {testing ? "Testando…" : "Testar"}
        </button>
      </div>
      {error ? <div className="rules__error">{error}</div> : null}
      {result ? (
        <div className="rules__tester-result">
          <div className={`rules__outcome rules__outcome--${result.outcome}`}>{result.outcome}</div>
          <pre className="rules__pre numeric">{JSON.stringify(result.target_spec, null, 2)}</pre>
          <pre className="rules__pre numeric">{result.describe_miss}</pre>
        </div>
      ) : null}
    </Panel>
  );
}

export function Rules() {
  const [file, setFile] = useState<RuleFile | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveOk, setSaveOk] = useState(false);
  const [expanded, setExpanded] = useState<Set<number>>(new Set());

  const load = async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const f = await getRules();
      setFile(f);
    } catch (err) {
      setLoadError(err instanceof ApiError ? err.message : "Falha ao carregar regras.");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, []);

  const updateRules = (next: Rule[]) => {
    if (!file) return;
    setFile({ ...file, rules: next });
    setSaveOk(false);
  };

  const save = async () => {
    if (!file) return;
    setSaving(true);
    setSaveError(null);
    setSaveOk(false);
    try {
      const saved = await putRules(file);
      setFile(saved);
      setSaveOk(true);
    } catch (err) {
      setSaveError(err instanceof ApiError ? err.message : "Falha ao salvar regras.");
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <Panel>
        <div className="dashboard__loading">Carregando regras…</div>
      </Panel>
    );
  }

  if (loadError || !file) {
    return (
      <div className="dashboard__banner" role="alert">
        <span>{loadError ?? "Falha ao carregar regras."}</span>
        <button type="button" onClick={load}>
          Tentar novamente
        </button>
      </div>
    );
  }

  return (
    <div className="rules">
      <div className="rules__toolbar">
        <h1 className="font-display rules__title">Regras</h1>
        <div className="rules__toolbar-actions">
          <button type="button" onClick={() => updateRules([...file.rules, blankRule()])}>
            + Adicionar regra
          </button>
          <button type="button" className="rules__save" onClick={save} disabled={saving}>
            {saving ? "Salvando…" : "Salvar"}
          </button>
        </div>
      </div>

      {saveError ? (
        <div className="dashboard__banner" role="alert">
          <span>{saveError}</span>
        </div>
      ) : null}
      {saveOk ? <div className="rules__save-ok">Regras salvas e recarregadas no servidor.</div> : null}

      <Panel title={`Lista de regras (${file.rules.length})`}>
        {file.rules.length === 0 ? (
          <span className="dashboard__empty-note">Nenhuma regra ainda. Adicione uma para começar.</span>
        ) : (
          <div className="rules__list">
            {file.rules.map((rule, i) => (
              <RuleCard
                key={i}
                rule={rule}
                index={i}
                total={file.rules.length}
                expanded={expanded.has(i)}
                onToggleExpanded={() =>
                  setExpanded((prev) => {
                    const next = new Set(prev);
                    if (next.has(i)) next.delete(i);
                    else next.add(i);
                    return next;
                  })
                }
                onChange={(next) => {
                  const copy = file.rules.slice();
                  copy[i] = next;
                  updateRules(copy);
                }}
                onRemove={() => {
                  const copy = file.rules.slice();
                  copy.splice(i, 1);
                  updateRules(copy);
                }}
                onMove={(delta) => {
                  const j = i + delta;
                  if (j < 0 || j >= file.rules.length) return;
                  const copy = file.rules.slice();
                  [copy[i], copy[j]] = [copy[j], copy[i]];
                  updateRules(copy);
                }}
              />
            ))}
          </div>
        )}
      </Panel>

      <GlobalConfigPanel file={file} />
      <RuleTester savedFile={saveOk ? file : null} />
    </div>
  );
}
