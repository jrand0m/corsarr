// Pure formatters for quantities and time display (timestamps, countdowns). Locale is fixed to en for v1
// (strings live in one catalogue); numbers are rendered with tabular figures by CSS.

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'] as const;

/** Bytes in decimal (SI) units, which match what download clients and disks report. */
export function formatBytes(bytes: number, digits = 1): string {
  if (!Number.isFinite(bytes)) return '—';
  const neg = bytes < 0;
  let v = Math.abs(bytes);
  let i = 0;
  while (v >= 1000 && i < BYTE_UNITS.length - 1) { v /= 1000; i++; }
  const d = i === 0 || v >= 100 ? 0 : digits;
  return `${neg ? '−' : ''}${v.toFixed(d)} ${BYTE_UNITS[i]}`;
}

export function formatSpeed(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond)) return '—';
  return `${formatBytes(bytesPerSecond)}/s`;
}

/** GB per hour of runtime: the size budget quality profiles are tuned with. */
export function formatGbPerHour(bytes: number, runtimeSeconds: number): string {
  if (!(runtimeSeconds > 0) || !Number.isFinite(bytes)) return '—';
  return `${(bytes / 1e9 / (runtimeSeconds / 3600)).toFixed(1)} GB/h`;
}

/** Compact duration: "45s", "12m", "3h 20m", "2d 4h". Rounds down; < 1s is "0s". */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds)) return '—';
  const s = Math.max(0, Math.floor(seconds));
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  if (d > 0) return h ? `${d}d ${h}h` : `${d}d`;
  if (h > 0) return m ? `${h}h ${m}m` : `${h}h`;
  if (m > 0) return `${m}m`;
  return `${s}s`;
}

/** Clock-style duration for countdowns: "4:05", "1:02:09". */
export function formatClock(seconds: number): string {
  const s = Math.max(0, Math.ceil(seconds));
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), r = s % 60;
  const mm = h ? String(m).padStart(2, '0') : String(m);
  return `${h ? `${h}:` : ''}${mm}:${String(r).padStart(2, '0')}`;
}

export function formatCount(n: number, limit?: number): string {
  const f = (x: number) => x.toLocaleString('en');
  return limit === undefined ? f(n) : `${f(n)} of ${f(limit)}`;
}

export function formatPercent(fraction: number, digits = 0): string {
  if (!Number.isFinite(fraction)) return '—';
  return `${(Math.min(1, Math.max(0, fraction)) * 100).toFixed(digits)}%`;
}
