// Relative and absolute timestamps. Pure: callers pass `now`.

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 31536000], ['month', 2592000], ['week', 604800], ['day', 86400], ['hour', 3600], ['minute', 60],
];
const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto', style: 'narrow' });

/** "just now", "5 min. ago", "in 3 hr.", "yesterday". */
export function formatRelative(date: Date, now: Date): string {
  const diff = (date.getTime() - now.getTime()) / 1000;
  const abs = Math.abs(diff);
  if (abs < 45) return 'just now';
  for (const [unit, secs] of UNITS) {
    if (abs >= secs || unit === 'minute') return rtf.format(Math.round(diff / secs), unit);
  }
  return 'just now';
}

const abs = new Intl.DateTimeFormat('en-GB', {
  year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false,
});
/** "22 Nov 2026, 14:05" in the browser's time zone. */
export function formatAbsolute(date: Date): string {
  return abs.format(date);
}

export function toDate(value: Date | string | number): Date {
  return value instanceof Date ? value : new Date(value);
}

const clock = new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit', hour12: false });
const day = new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short' });
/** Axis label: "14:00" for spans under two days, "22 Nov" beyond. */
export function formatAxisTime(date: Date, spanSeconds: number): string {
  return spanSeconds < 2 * 86400 ? clock.format(date) : day.format(date);
}
/** Compact stamp for chart tooltips: "22 Nov, 14:00". */
export function formatShortStamp(date: Date): string {
  return `${day.format(date)}, ${clock.format(date)}`;
}
