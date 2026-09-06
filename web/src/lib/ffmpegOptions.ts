export interface OptionItem {
  value: string;
  label: string;
  hint?: string;
}

export const VIDEO_CODEC_OPTIONS: OptionItem[] = [
  { value: "libx265", label: "libx265 (HEVC / H.265)", hint: "Recomendado para equilíbrio entre eficiência e compatibilidade" },
  { value: "hevc", label: "hevc (HEVC genérico / HW)", hint: "Codificador H.265 (automático pelo driver/hardware)" },
  { value: "libsvtav1", label: "libsvtav1 (AV1 - SVT)", hint: "Estado da arte em compressão e padrões abertos" },
  { value: "libaom-av1", label: "libaom-av1 (AV1 - AOM)", hint: "Codificador de referência AV1 (ótimo para lossless)" },
  { value: "libx264", label: "libx264 (AVC / H.264)", hint: "Compatibilidade universal com players e dispositivos antigos" },
  { value: "copy", label: "copy (Stream Copy)", hint: "Não reencodifica o vídeo; apenas copia o fluxo original" },
  { value: "libvpx-vp9", label: "libvpx-vp9 (VP9)", hint: "Codec aberto do Google, muito usado para web" },
  { value: "prores", label: "prores (Apple ProRes)", hint: "Codec intermediário para edição profissional" },
];

export const SVT_AV1_PRESET_OPTIONS: OptionItem[] = [
  { value: "3", label: "3 (SVT-AV1 - Máxima Eficiência)", hint: "Extremamente lento, máxima retenção de detalhes" },
  { value: "4", label: "4 (SVT-AV1 - Lento / Arquivamento)", hint: "Alta qualidade e alta compressão (Recomendado Anime/Cinema)" },
  { value: "5", label: "5 (SVT-AV1 - Equilibrado / Padrão)", hint: "Excelente equilíbrio entre velocidade e tamanho" },
  { value: "6", label: "6 (SVT-AV1 - Rápido)", hint: "Conversão ágil com ótima retenção de qualidade" },
  { value: "7", label: "7 (SVT-AV1 - Muito Rápido)", hint: "Mais rápido, compressão moderada" },
  { value: "8", label: "8 (SVT-AV1 - Ultra Rápido)", hint: "Foco total em velocidade de encode" },
];

export const NVENC_PRESET_OPTIONS: OptionItem[] = [
  { value: "p1", label: "p1 (NVENC - Mais rápido)", hint: "Preset NVENC mais rápido" },
  { value: "p2", label: "p2 (NVENC - Muito rápido)", hint: "Foco em velocidade" },
  { value: "p3", label: "p3 (NVENC - Rápido)", hint: "Rápido" },
  { value: "p4", label: "p4 (NVENC - Equilibrado / Padrão)", hint: "Preset NVENC equilibrado" },
  { value: "p5", label: "p5 (NVENC - Boa qualidade)", hint: "Otimizado para qualidade" },
  { value: "p6", label: "p6 (NVENC - Alta qualidade)", hint: "Preset NVENC otimizado para qualidade" },
  { value: "p7", label: "p7 (NVENC - Máxima qualidade)", hint: "Preset NVENC mais lento e refinado" },
];

export const CPU_PRESET_OPTIONS: OptionItem[] = [
  { value: "ultrafast", label: "ultrafast", hint: "Mais rápido possível, menor taxa de compressão" },
  { value: "superfast", label: "superfast", hint: "Muito rápido" },
  { value: "veryfast", label: "veryfast", hint: "Rápido com compressão razoável" },
  { value: "faster", label: "faster", hint: "Mais rápido que o padrão" },
  { value: "fast", label: "fast", hint: "Bom equilíbrio com ênfase em velocidade" },
  { value: "medium", label: "medium (Padrão)", hint: "Padrão recomendado para x264/x265" },
  { value: "slow", label: "slow (Recomendado x265/x264)", hint: "Melhor eficiência de compressão" },
  { value: "slower", label: "slower", hint: "Alta compressão, processamento lento" },
  { value: "veryslow", label: "veryslow", hint: "Máxima eficiência de compressão por CPU" },
];

export const PRESET_OPTIONS: OptionItem[] = [
  ...CPU_PRESET_OPTIONS,
  ...NVENC_PRESET_OPTIONS,
  ...SVT_AV1_PRESET_OPTIONS,
];

export const SVT_AV1_TUNE_OPTIONS: OptionItem[] = [
  { value: "0", label: "0 (Qualidade Visual / VQ - Padrão)", hint: "Otimização psicovisual para percepção humana" },
  { value: "1", label: "1 (PSNR Sintético)", hint: "Otimização sintética para métricas e benchmarks" },
  { value: "2", label: "2 (SSIM Sintético)", hint: "Otimização sintética para métricas SSIM" },
];

export const CPU_TUNE_OPTIONS: OptionItem[] = [
  { value: "", label: "Padrão / Nenhum", hint: "Comportamento geral balanceado do codificador" },
  { value: "film", label: "film (Filmes / Live-action)", hint: "Preserva texturas e granulação natural de filmes reais" },
  { value: "animation", label: "animation (Desenhos / Animes)", hint: "Otimizado para traços nítidos e áreas de cores sólidas 2D" },
  { value: "grain", label: "grain (Película / Granulação)", hint: "Mantém a estrutura de grão analógico (película 35mm/16mm)" },
  { value: "stillimage", label: "stillimage (Imagens estáticas)", hint: "Otimizado para fotos, slides e pouco movimento" },
  { value: "fastdecode", label: "fastdecode (Decodificação rápida)", hint: "Reduz esforço de CPU do reprodutor/dispositivo" },
  { value: "zerolatency", label: "zerolatency (Tempo real / Streaming)", hint: "Elimina buffers para latência mínima" },
];

export const TUNE_OPTIONS: OptionItem[] = [
  ...CPU_TUNE_OPTIONS,
  ...SVT_AV1_TUNE_OPTIONS,
];

export function getPresetsForCodec(codec: string = "", hwaccel: string = ""): OptionItem[] {
  const normCodec = codec.toLowerCase();
  const normHw = hwaccel.toLowerCase();

  if (normHw === "nvenc" || normCodec.includes("nvenc")) {
    return NVENC_PRESET_OPTIONS;
  }
  if (normCodec.includes("av1") || normCodec.includes("svt")) {
    return SVT_AV1_PRESET_OPTIONS;
  }
  return CPU_PRESET_OPTIONS;
}

export function getPresetChipsForCodec(codec: string = "", hwaccel: string = ""): { val: string; label: string }[] {
  const normCodec = codec.toLowerCase();
  const normHw = hwaccel.toLowerCase();

  if (normHw === "nvenc" || normCodec.includes("nvenc")) {
    return [
      { val: "p1", label: "p1 (Rápido)" },
      { val: "p4", label: "p4 (Equilibrado)" },
      { val: "p6", label: "p6 (Alta Qualidade)" },
      { val: "p7", label: "p7 (Máximo)" },
    ];
  }
  if (normCodec.includes("av1") || normCodec.includes("svt")) {
    return [
      { val: "4", label: "4 (Lento / Alta Comp)" },
      { val: "5", label: "5 (Equilibrado)" },
      { val: "6", label: "6 (Rápido)" },
      { val: "8", label: "8 (Ultra Rápido)" },
    ];
  }
  return [
    { val: "faster", label: "faster" },
    { val: "medium", label: "medium" },
    { val: "slow", label: "slow" },
    { val: "slower", label: "slower" },
  ];
}

export function getTunesForCodec(codec: string = ""): OptionItem[] {
  const normCodec = codec.toLowerCase();
  if (normCodec.includes("av1") || normCodec.includes("svt")) {
    return SVT_AV1_TUNE_OPTIONS;
  }
  return CPU_TUNE_OPTIONS;
}

export function getTuneChipsForCodec(codec: string = ""): { val: string; label: string }[] {
  const normCodec = codec.toLowerCase();
  if (normCodec.includes("av1") || normCodec.includes("svt")) {
    return [
      { val: "0", label: "0 (Qualidade Visual / VQ)" },
      { val: "1", label: "1 (PSNR)" },
      { val: "2", label: "2 (SSIM)" },
    ];
  }
  return [
    { val: "", label: "nenhum" },
    { val: "film", label: "film" },
    { val: "animation", label: "animation" },
    { val: "grain", label: "grain" },
  ];
}

export const HWACCEL_OPTIONS: OptionItem[] = [
  { value: "", label: "Nenhuma (CPU / Software)", hint: "Maior eficiência de compressão e qualidade visual" },
  { value: "nvenc", label: "nvenc (NVIDIA GPU)", hint: "Aceleração rápida via placas GeForce/Quadro/RTX" },
  { value: "qsv", label: "qsv (Intel Quick Sync)", hint: "Aceleração via processadores/GPUs Intel" },
  { value: "vaapi", label: "vaapi (Linux Genérico / AMD / Intel)", hint: "API nativa Linux para aceleração de vídeo" },
  { value: "videotoolbox", label: "videotoolbox (Apple Silicon / macOS)", hint: "Aceleração de hardware nativa no macOS" },
  { value: "amf", label: "amf (AMD GPU)", hint: "Advanced Media Framework da AMD" },
];

export const CONTAINER_OPTIONS: OptionItem[] = [
  { value: "mkv", label: "mkv (Matroska)", hint: "Suporte completo a múltiplos áudios, legendas e codecs modernos" },
  { value: "mp4", label: "mp4 (MPEG-4 Part 14)", hint: "Compatibilidade máxima em reprodutores de mídia e navegadores" },
  { value: "webm", label: "webm", hint: "Otimizado para web (VP9/AV1 + Opus)" },
  { value: "mov", label: "mov (QuickTime)", hint: "Comum em ecossistema Apple e edição profissional" },
  { value: "auto", label: "auto (Manter contêiner original)", hint: "Herda a extensão de contêiner da fonte" },
];

export const AUDIO_CODEC_OPTIONS: OptionItem[] = [
  { value: "copy", label: "copy (Stream Copy)", hint: "Mantém a faixa de áudio idêntica à original (sem reencode)" },
  { value: "aac", label: "aac (Advanced Audio Coding)", hint: "Padrão universal, ótimo balanço de compatibilidade e qualidade" },
  { value: "libopus", label: "libopus / opus", hint: "Melhor qualidade por bitrate do mercado moderno" },
  { value: "flac", label: "flac (Lossless)", hint: "Áudio sem perdas matematicamente preservado" },
  { value: "ac3", label: "ac3 (Dolby Digital 5.1)", hint: "Compatibilidade ampla com soundbars e home theaters" },
  { value: "eac3", label: "eac3 (Dolby Digital Plus)", hint: "Áudio surround de alta taxa de bits" },
  { value: "mp3", label: "mp3 (libmp3lame)", hint: "Formato clássico de ampla compatibilidade" },
];

export const AUDIO_BITRATE_OPTIONS = [
  "64k",
  "96k",
  "128k",
  "160k",
  "192k",
  "256k",
  "320k",
];

export const RESOLUTION_PRESETS = [
  { label: "Original (0)", height: 0 },
  { label: "480p", height: 480 },
  { label: "720p", height: 720 },
  { label: "1080p", height: 1080 },
  { label: "1440p", height: 1440 },
  { label: "4K (2160p)", height: 2160 },
];

export interface CrfInfo {
  tier: "lossless" | "studio" | "high" | "balanced" | "compression" | "extreme";
  label: string;
  badgeClass: string;
  description: string;
  codecAdvice?: string;
}

export function evaluateCrf(crf: number, codec: string = ""): CrfInfo {
  const normCodec = codec.toLowerCase();
  const isAv1 = normCodec.includes("av1") || normCodec.includes("svt");
  const isX265 = normCodec.includes("265") || normCodec.includes("hevc");
  const isX264 = normCodec.includes("264") || normCodec.includes("avc");

  let codecAdvice = "";
  if (isAv1) {
    codecAdvice = "No SVT-AV1, a faixa recomendada para excelente fidelidade e alta economia é CRF 26–34.";
  } else if (isX265) {
    codecAdvice = "No H.265 / libx265, a faixa recomendada para transparência visual é CRF 18–24.";
  } else if (isX264) {
    codecAdvice = "No H.264 / libx264, a faixa clássica recomendada é CRF 18–23.";
  }

  if (crf === 0) {
    return {
      tier: "lossless",
      label: "CRF 0 • Sem perdas / Padrão",
      badgeClass: "rules__crf-badge--lossless",
      description: "Modo sem perdas (lossless) ou valor padrão do encoder.",
      codecAdvice,
    };
  }

  if (crf <= 17) {
    return {
      tier: "studio",
      label: `CRF ${crf} • Qualidade de Estúdio`,
      badgeClass: "rules__crf-badge--studio",
      description: "Fidelidade quase matemática. Gera arquivos consideravelmente grandes.",
      codecAdvice,
    };
  }

  if (crf <= 22) {
    return {
      tier: "high",
      label: `CRF ${crf} • Alta Fidelidade / Preservação`,
      badgeClass: "rules__crf-badge--high",
      description: "Excelente para arquivamento. Preserva grãos finos e detalhes complexos.",
      codecAdvice,
    };
  }

  if (crf <= 28) {
    return {
      tier: "balanced",
      label: `CRF ${crf} • Equilibrado`,
      badgeClass: "rules__crf-badge--balanced",
      description: "Ótimo ponto de equilíbrio entre redução de espaço e retenção de detalhes.",
      codecAdvice,
    };
  }

  if (crf <= 35) {
    return {
      tier: "compression",
      label: `CRF ${crf} • Alta Compressão`,
      badgeClass: "rules__crf-badge--compression",
      description: "Foco em menor tamanho de arquivo. Excelente para AV1 ou conteúdo leve.",
      codecAdvice,
    };
  }

  return {
    tier: "extreme",
    label: `CRF ${crf} • Compressão Extrema`,
    badgeClass: "rules__crf-badge--extreme",
    description: "Arquivos muito pequenos, mas com artefatos visuais e perda de textura perceptível.",
    codecAdvice,
  };
}
