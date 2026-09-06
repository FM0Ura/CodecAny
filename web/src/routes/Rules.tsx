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
import {
  AUDIO_BITRATE_OPTIONS,
  AUDIO_CODEC_OPTIONS,
  CONTAINER_OPTIONS,
  evaluateCrf,
  HWACCEL_OPTIONS,
  PRESET_OPTIONS,
  RESOLUTION_PRESETS,
  TUNE_OPTIONS,
  VIDEO_CODEC_OPTIONS,
} from "../lib/ffmpegOptions";
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
  tune: "",
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

/** Datalists globais para autocompletar sugestões padrão do FFmpeg com fallback livre */
function FfmpegDatalists() {
  return (
    <>
      <datalist id="datalist-video-codecs">
        {VIDEO_CODEC_OPTIONS.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label} — {o.hint}
          </option>
        ))}
      </datalist>
      <datalist id="datalist-presets">
        {PRESET_OPTIONS.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label} ({o.hint})
          </option>
        ))}
      </datalist>
      <datalist id="datalist-tunes">
        {TUNE_OPTIONS.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label} — {o.hint}
          </option>
        ))}
      </datalist>
      <datalist id="datalist-hwaccels">
        {HWACCEL_OPTIONS.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label} — {o.hint}
          </option>
        ))}
      </datalist>
      <datalist id="datalist-containers">
        {CONTAINER_OPTIONS.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label} — {o.hint}
          </option>
        ))}
      </datalist>
      <datalist id="datalist-audio-codecs">
        {AUDIO_CODEC_OPTIONS.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label} — {o.hint}
          </option>
        ))}
      </datalist>
      <datalist id="datalist-audio-bitrates">
        {AUDIO_BITRATE_OPTIONS.map((br) => (
          <option key={br} value={br}>
            {br}
          </option>
        ))}
      </datalist>
    </>
  );
}

/** Botão de três estados (herdar / sim / não) para campos *bool */
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

/** Seletor de Resolução com Chips de Acesso Rápido */
function ResolutionPicker({
  label,
  value,
  onChange,
  hint,
}: {
  label: string;
  value: number;
  onChange: (v: number) => void;
  hint?: string;
}) {
  return (
    <div className="rules__field-block">
      <label className="rules__field-label">
        {label}
        {hint && <span className="rules__field-hint">{hint}</span>}
      </label>
      <div className="rules__input-with-chips">
        <input
          type="number"
          min={0}
          step={2}
          className="numeric rules__input"
          value={value || 0}
          onChange={(e) => onChange(Math.max(0, Number(e.target.value) || 0))}
          placeholder="0 (qualquer)"
        />
        <div className="rules__chips-row">
          {RESOLUTION_PRESETS.map((p) => (
            <button
              key={p.height}
              type="button"
              className={`rules__chip ${value === p.height ? "rules__chip--active" : ""}`}
              onClick={() => onChange(p.height)}
            >
              {p.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

/** Controle de Qualidade CRF visual e interativo */
function CrfQualityControl({
  value,
  codec,
  onChange,
}: {
  value: number;
  codec?: string;
  onChange: (v: number) => void;
}) {
  const crf = value || 0;
  const info = evaluateCrf(crf, codec);

  return (
    <div className="rules__crf-card">
      <div className="rules__crf-head">
        <div className="rules__crf-titles">
          <label className="rules__field-label">video.crf (Qualidade / Constant Rate Factor)</label>
          <span className="rules__field-hint">Escala de compressão perceptiva do encoder</span>
        </div>
        <div className="rules__crf-input-group">
          <input
            type="number"
            min={0}
            max={63}
            className="numeric rules__crf-number-input"
            value={crf}
            onChange={(e) => onChange(Math.max(0, Math.min(63, Number(e.target.value) || 0)))}
          />
        </div>
      </div>

      <div className="rules__crf-slider-section">
        <input
          type="range"
          min={0}
          max={51}
          step={1}
          value={crf > 51 ? 51 : crf}
          className="rules__crf-slider"
          onChange={(e) => onChange(Number(e.target.value))}
        />
        <div className="rules__crf-markers">
          <span onClick={() => onChange(0)} title="0: Padrão / Lossless">0 (Lossless)</span>
          <span onClick={() => onChange(18)} title="18: Alta Fidelidade">18</span>
          <span onClick={() => onChange(22)} title="22: Preservação">22</span>
          <span onClick={() => onChange(28)} title="28: Equilibrado">28</span>
          <span onClick={() => onChange(34)} title="34: Compacto">34</span>
          <span onClick={() => onChange(45)} title="45+: Extremo">45+</span>
        </div>
      </div>

      <div className="rules__chips-row rules__crf-quick-chips">
        <span className="rules__chips-label">Atalhos:</span>
        <button
          type="button"
          className={`rules__chip ${crf === 0 ? "rules__chip--active" : ""}`}
          onClick={() => onChange(0)}
        >
          0 (Lossless)
        </button>
        <button
          type="button"
          className={`rules__chip ${crf === 18 ? "rules__chip--active" : ""}`}
          onClick={() => onChange(18)}
        >
          18 (Fidelidade máx)
        </button>
        <button
          type="button"
          className={`rules__chip ${crf === 22 ? "rules__chip--active" : ""}`}
          onClick={() => onChange(22)}
        >
          22 (H.265 Padrão)
        </button>
        <button
          type="button"
          className={`rules__chip ${crf === 28 ? "rules__chip--active" : ""}`}
          onClick={() => onChange(28)}
        >
          28 (Equilibrado / AV1)
        </button>
        <button
          type="button"
          className={`rules__chip ${crf === 32 ? "rules__chip--active" : ""}`}
          onClick={() => onChange(32)}
        >
          32 (AV1 Econômico)
        </button>
      </div>

      <div className={`rules__crf-feedback ${info.badgeClass}`}>
        <div className="rules__crf-badge-title">{info.label}</div>
        <div className="rules__crf-badge-desc">{info.description}</div>
      </div>

      {info.codecAdvice && (
        <div className="rules__crf-advice">
          <span className="rules__crf-advice-icon">💡</span>
          <span>{info.codecAdvice}</span>
        </div>
      )}
    </div>
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
          <button
            type="button"
            disabled={index === 0}
            onClick={() => onMove(-1)}
            title="Mover para cima"
            aria-label="Mover regra para cima"
          >
            ▲
          </button>
          <button
            type="button"
            disabled={index === total - 1}
            onClick={() => onMove(1)}
            title="Mover para baixo"
            aria-label="Mover regra para baixo"
          >
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
          <option value="convert">convert (converter)</option>
          <option value="skip">skip (ignorar)</option>
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
          {/* Match (Condições de Origem) */}
          <fieldset className="rule-card__fieldset">
            <legend className="rule-card__legend">Match (Condições do Arquivo de Origem)</legend>
            <div className="rule-card__grid">
              <div className="rules__field-block">
                <label className="rules__field-label">
                  Container (origem)
                  <span className="rules__field-hint">Extensões separadas por vírgula</span>
                </label>
                <input
                  className="numeric rules__input"
                  list="datalist-containers"
                  value={itemToText(rule.match.container)}
                  placeholder="ex: mkv, mp4, avi"
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      match: updateMatch(rule.match, { container: itemFromText(e.target.value) }),
                    })
                  }
                />
                <div className="rules__chips-row">
                  {["mkv", "mp4", "avi", "ts", "mov"].map((c) => {
                    const current = itemValues(rule.match.container);
                    const active = current.includes(c);
                    return (
                      <button
                        key={c}
                        type="button"
                        className={`rules__chip ${active ? "rules__chip--active" : ""}`}
                        onClick={() => {
                          const next = active ? current.filter((x) => x !== c) : [...current, c];
                          onChange({
                            ...rule,
                            match: updateMatch(rule.match, { container: next }),
                          });
                        }}
                      >
                        {active ? `✓ ${c}` : `+ ${c}`}
                      </button>
                    );
                  })}
                </div>
              </div>

              <div className="rules__field-block">
                <label className="rules__field-label">
                  video.codec (origem)
                  <span className="rules__field-hint">Codecs separados por vírgula</span>
                </label>
                <input
                  className="numeric rules__input"
                  list="datalist-video-codecs"
                  value={itemToText(rule.match.video.codec)}
                  placeholder="ex: h264, mpeg2video, vc1"
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      match: updateMatch(rule.match, {
                        video: { ...rule.match.video, codec: itemFromText(e.target.value) },
                      }),
                    })
                  }
                />
                <div className="rules__chips-row">
                  {["h264", "hevc", "mpeg2video", "vc1"].map((c) => {
                    const current = itemValues(rule.match.video.codec);
                    const active = current.includes(c);
                    return (
                      <button
                        key={c}
                        type="button"
                        className={`rules__chip ${active ? "rules__chip--active" : ""}`}
                        onClick={() => {
                          const next = active ? current.filter((x) => x !== c) : [...current, c];
                          onChange({
                            ...rule,
                            match: updateMatch(rule.match, {
                              video: { ...rule.match.video, codec: next },
                            }),
                          });
                        }}
                      >
                        {active ? `✓ ${c}` : `+ ${c}`}
                      </button>
                    );
                  })}
                </div>
              </div>

              <ResolutionPicker
                label="video.min_height (altura mínima)"
                hint="Ex: filtrar apenas vídeos >= 1080p"
                value={rule.match.video.min_height}
                onChange={(v) =>
                  onChange({
                    ...rule,
                    match: updateMatch(rule.match, {
                      video: { ...rule.match.video, min_height: v },
                    }),
                  })
                }
              />

              <ResolutionPicker
                label="video.max_height (altura máxima)"
                hint="Ex: filtrar vídeos até 720p"
                value={rule.match.video.max_height}
                onChange={(v) =>
                  onChange({
                    ...rule,
                    match: updateMatch(rule.match, {
                      video: { ...rule.match.video, max_height: v },
                    }),
                  })
                }
              />

              <div className="rules__field-block">
                <label className="rules__field-label">
                  video.min_bitrate_kbps
                  <span className="rules__field-hint">Bitrate mínimo do vídeo de origem</span>
                </label>
                <input
                  type="number"
                  min={0}
                  step={100}
                  className="numeric rules__input"
                  value={rule.match.video.min_bitrate_kbps || 0}
                  placeholder="0 (sem limite)"
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      match: updateMatch(rule.match, {
                        video: {
                          ...rule.match.video,
                          min_bitrate_kbps: Math.max(0, Number(e.target.value) || 0),
                        },
                      }),
                    })
                  }
                />
                <div className="rules__chips-row">
                  {[0, 2000, 4000, 8000, 15000].map((br) => (
                    <button
                      key={br}
                      type="button"
                      className={`rules__chip ${rule.match.video.min_bitrate_kbps === br ? "rules__chip--active" : ""}`}
                      onClick={() =>
                        onChange({
                          ...rule,
                          match: updateMatch(rule.match, {
                            video: { ...rule.match.video, min_bitrate_kbps: br },
                          }),
                        })
                      }
                    >
                      {br === 0 ? "0 (qualquer)" : `${br} kbps`}
                    </button>
                  ))}
                </div>
              </div>

              <div className="rules__field-block">
                <label className="rules__field-label">
                  audio.codec (origem)
                  <span className="rules__field-hint">Opcional / Não-bloqueante</span>
                </label>
                <input
                  className="numeric rules__input"
                  list="datalist-audio-codecs"
                  value={itemToText(rule.match.audio.codec)}
                  placeholder="ex: aac, ac3, dts, flac"
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      match: updateMatch(rule.match, {
                        audio: { codec: itemFromText(e.target.value) },
                      }),
                    })
                  }
                />
                <div className="rules__chips-row">
                  {["aac", "ac3", "dts", "flac", "mp3"].map((c) => {
                    const current = itemValues(rule.match.audio.codec);
                    const active = current.includes(c);
                    return (
                      <button
                        key={c}
                        type="button"
                        className={`rules__chip ${active ? "rules__chip--active" : ""}`}
                        onClick={() => {
                          const next = active ? current.filter((x) => x !== c) : [...current, c];
                          onChange({
                            ...rule,
                            match: updateMatch(rule.match, { audio: { codec: next } }),
                          });
                        }}
                      >
                        {active ? `✓ ${c}` : `+ ${c}`}
                      </button>
                    );
                  })}
                </div>
              </div>
            </div>
          </fieldset>

          {/* Convert (Destino / Especificação Alvo) */}
          <fieldset className="rule-card__fieldset rule-card__fieldset--convert">
            <legend className="rule-card__legend">Convert (Configurações do FFmpeg Alvo)</legend>

            <div className="rules__section-title">Parâmetros de Vídeo</div>
            <div className="rule-card__grid">
              <div className="rules__field-block">
                <label className="rules__field-label">
                  video.codec
                  <span className="rules__field-hint">Codificador de vídeo de saída</span>
                </label>
                <input
                  className="numeric rules__input"
                  list="datalist-video-codecs"
                  value={video?.codec ?? ""}
                  placeholder="ex: libx265, libsvtav1, copy"
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        video: { ...(video ?? DEFAULT_TARGET_VIDEO), codec: e.target.value },
                      }),
                    })
                  }
                />
                <div className="rules__chips-row">
                  {[
                    { label: "libx265 (H.265)", val: "libx265" },
                    { label: "libsvtav1 (AV1)", val: "libsvtav1" },
                    { label: "libx264 (H.264)", val: "libx264" },
                    { label: "copy (manter)", val: "copy" },
                  ].map((c) => (
                    <button
                      key={c.val}
                      type="button"
                      className={`rules__chip ${video?.codec === c.val ? "rules__chip--active" : ""}`}
                      onClick={() =>
                        onChange({
                          ...rule,
                          convert: updateConvert(rule.convert, {
                            video: { ...(video ?? DEFAULT_TARGET_VIDEO), codec: c.val },
                          }),
                        })
                      }
                    >
                      {c.label}
                    </button>
                  ))}
                </div>
              </div>

              <div className="rules__field-block">
                <label className="rules__field-label">
                  video.preset
                  <span className="rules__field-hint">Velocidade vs. eficiência de compressão</span>
                </label>
                <input
                  className="numeric rules__input"
                  list="datalist-presets"
                  value={video?.preset ?? ""}
                  placeholder="ex: medium, slow, p6, 6"
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        video: { ...(video ?? DEFAULT_TARGET_VIDEO), preset: e.target.value },
                      }),
                    })
                  }
                />
                <div className="rules__chips-row">
                  {["medium", "slow", "faster", "p6", "6"].map((p) => (
                    <button
                      key={p}
                      type="button"
                      className={`rules__chip ${video?.preset === p ? "rules__chip--active" : ""}`}
                      onClick={() =>
                        onChange({
                          ...rule,
                          convert: updateConvert(rule.convert, {
                            video: { ...(video ?? DEFAULT_TARGET_VIDEO), preset: p },
                          }),
                        })
                      }
                    >
                      {p}
                    </button>
                  ))}
                </div>
              </div>

              <div className="rules__field-block">
                <label className="rules__field-label">
                  video.tune
                  <span className="rules__field-hint">Otimização para tipo de conteúdo ou métrica</span>
                </label>
                <input
                  className="numeric rules__input"
                  list="datalist-tunes"
                  value={video?.tune ?? ""}
                  placeholder="ex: film, animation, grain, 0"
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        video: { ...(video ?? DEFAULT_TARGET_VIDEO), tune: e.target.value },
                      }),
                    })
                  }
                />
                <div className="rules__chips-row">
                  {[
                    { label: "film", val: "film" },
                    { label: "animation", val: "animation" },
                    { label: "grain", val: "grain" },
                    { label: "stillimage", val: "stillimage" },
                    { label: "fastdecode", val: "fastdecode" },
                  ].map((t) => (
                    <button
                      key={t.val}
                      type="button"
                      className={`rules__chip ${video?.tune === t.val ? "rules__chip--active" : ""}`}
                      onClick={() =>
                        onChange({
                          ...rule,
                          convert: updateConvert(rule.convert, {
                            video: {
                              ...(video ?? DEFAULT_TARGET_VIDEO),
                              tune: video?.tune === t.val ? "" : t.val,
                            },
                          }),
                        })
                      }
                    >
                      {t.label}
                    </button>
                  ))}
                </div>
              </div>

              <div className="rules__field-block">
                <label className="rules__field-label">
                  video.hwaccel
                  <span className="rules__field-hint">Aceleração de codificação via GPU</span>
                </label>
                <select
                  className="rules__select"
                  value={video?.hwaccel ?? ""}
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        video: { ...(video ?? DEFAULT_TARGET_VIDEO), hwaccel: e.target.value },
                      }),
                    })
                  }
                >
                  {HWACCEL_OPTIONS.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
              </div>

              <ResolutionPicker
                label="video.max_height (redimensionamento)"
                hint="Escala para baixo se a fonte for maior (0 = manter original)"
                value={video?.max_height ?? 0}
                onChange={(v) =>
                  onChange({
                    ...rule,
                    convert: updateConvert(rule.convert, {
                      video: { ...(video ?? DEFAULT_TARGET_VIDEO), max_height: v },
                    }),
                  })
                }
              />

              <div className="rules__field-block">
                <label className="rules__field-label">
                  video.lossless
                  <span className="rules__field-hint">Preservação matemática exata de pixels</span>
                </label>
                <TriStateSelect
                  value={video?.lossless ?? null}
                  trueLabel="Lossless (Sem perdas)"
                  falseLabel="Lossy (Com compressão perceptiva)"
                  onChange={(v) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        video: { ...(video ?? DEFAULT_TARGET_VIDEO), lossless: v },
                      }),
                    })
                  }
                />
              </div>
            </div>

            {/* Controle de CRF em largura completa com slider e feedback */}
            <div className="rules__crf-fullwidth">
              <CrfQualityControl
                value={video?.crf ?? 0}
                codec={video?.codec ?? ""}
                onChange={(v) =>
                  onChange({
                    ...rule,
                    convert: updateConvert(rule.convert, {
                      video: { ...(video ?? DEFAULT_TARGET_VIDEO), crf: v },
                    }),
                  })
                }
              />
            </div>

            <div className="rules__section-title">Parâmetros de Áudio</div>
            <div className="rule-card__grid">
              <div className="rules__field-block">
                <label className="rules__field-label">
                  audio.codec
                  <span className="rules__field-hint">Codec para as faixas de áudio</span>
                </label>
                <input
                  className="numeric rules__input"
                  list="datalist-audio-codecs"
                  value={audio?.codec ?? ""}
                  placeholder="ex: copy, aac, libopus"
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        audio: { codec: e.target.value, bitrate: audio?.bitrate ?? "" },
                      }),
                    })
                  }
                />
                <div className="rules__chips-row">
                  {[
                    { label: "copy (manter)", val: "copy" },
                    { label: "aac", val: "aac" },
                    { label: "opus", val: "libopus" },
                    { label: "flac (lossless)", val: "flac" },
                  ].map((c) => (
                    <button
                      key={c.val}
                      type="button"
                      className={`rules__chip ${audio?.codec === c.val ? "rules__chip--active" : ""}`}
                      onClick={() =>
                        onChange({
                          ...rule,
                          convert: updateConvert(rule.convert, {
                            audio: { codec: c.val, bitrate: audio?.bitrate ?? "" },
                          }),
                        })
                      }
                    >
                      {c.label}
                    </button>
                  ))}
                </div>
              </div>

              <div className="rules__field-block">
                <label className="rules__field-label">
                  audio.bitrate
                  <span className="rules__field-hint">Taxa de bits (ex: 128k, 192k)</span>
                </label>
                <input
                  className="numeric rules__input"
                  list="datalist-audio-bitrates"
                  value={audio?.bitrate ?? ""}
                  placeholder="ex: 128k, 192k"
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, {
                        audio: { codec: audio?.codec ?? "", bitrate: e.target.value },
                      }),
                    })
                  }
                />
                <div className="rules__chips-row">
                  {["96k", "128k", "192k", "256k", "320k"].map((br) => (
                    <button
                      key={br}
                      type="button"
                      className={`rules__chip ${audio?.bitrate === br ? "rules__chip--active" : ""}`}
                      onClick={() =>
                        onChange({
                          ...rule,
                          convert: updateConvert(rule.convert, {
                            audio: { codec: audio?.codec ?? "", bitrate: br },
                          }),
                        })
                      }
                    >
                      {br}
                    </button>
                  ))}
                </div>
              </div>
            </div>

            <div className="rules__section-title">Contêiner e Execução</div>
            <div className="rule-card__grid">
              <div className="rules__field-block">
                <label className="rules__field-label">
                  container
                  <span className="rules__field-hint">Extensão final do arquivo gerado</span>
                </label>
                <input
                  className="numeric rules__input"
                  list="datalist-containers"
                  value={rule.convert.container}
                  placeholder="auto (ou mkv, mp4)"
                  onChange={(e) =>
                    onChange({
                      ...rule,
                      convert: updateConvert(rule.convert, { container: e.target.value }),
                    })
                  }
                />
                <div className="rules__chips-row">
                  {["auto", "mkv", "mp4", "webm"].map((c) => (
                    <button
                      key={c}
                      type="button"
                      className={`rules__chip ${rule.convert.container === c ? "rules__chip--active" : ""}`}
                      onClick={() =>
                        onChange({
                          ...rule,
                          convert: updateConvert(rule.convert, { container: c }),
                        })
                      }
                    >
                      {c}
                    </button>
                  ))}
                </div>
              </div>

              <div className="rules__field-block">
                <label className="rules__field-label">
                  auto_approve
                  <span className="rules__field-hint">Aprovar conversão automaticamente sem confirmação</span>
                </label>
                <TriStateSelect
                  value={rule.auto_approve}
                  trueLabel="Auto-aprovar (executar direto)"
                  falseLabel="Requerer aprovação manual"
                  onChange={(v) => onChange({ ...rule, auto_approve: v })}
                />
              </div>
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
      <FfmpegDatalists />

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
