const UNITS = ["B", "KB", "MB", "GB", "TB"];

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes === 0) return "0 B";
  const sign = bytes < 0 ? "-" : "";
  let value = Math.abs(bytes);
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < UNITS.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  const decimals = unitIndex === 0 ? 0 : value < 10 ? 2 : 1;
  return `${sign}${value.toFixed(decimals)} ${UNITS[unitIndex]}`;
}

export function formatPct(pct: number): string {
  if (!Number.isFinite(pct)) return "0%";
  return `${pct.toFixed(1)}%`;
}

/** Nome do arquivo, sem o caminho completo. */
export function basename(path: string): string {
  const parts = path.split(/[/\\]/);
  return parts[parts.length - 1] || path;
}
