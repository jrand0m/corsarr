// Calendar maths and labels. Local time, weeks start on Monday by default. Pure.

const DAY = 86_400_000;

export function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}
export function addDays(d: Date, n: number): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
}
export function addMonths(d: Date, n: number): Date {
  return new Date(d.getFullYear(), d.getMonth() + n, 1);
}
export function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}
/** Monday (or `weekStartsOn`) of the week holding `d`. */
export function startOfWeek(d: Date, weekStartsOn = 1): Date {
  const s = startOfDay(d);
  return addDays(s, -((s.getDay() - weekStartsOn + 7) % 7));
}
/** The 6×7 (or 5×7) grid of days covering the month of `d`. */
export function monthGrid(d: Date, weekStartsOn = 1): Date[] {
  const first = new Date(d.getFullYear(), d.getMonth(), 1);
  const start = startOfWeek(first, weekStartsOn);
  const next = addMonths(first, 1);
  const weeks = Math.ceil((Math.round((next.getTime() - start.getTime()) / DAY)) / 7);
  return Array.from({ length: weeks * 7 }, (_, i) => addDays(start, i));
}
/** "YYYY-MM-DD" in local time, for URLs and keys. */
export function isoDay(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

const monthTitle = new Intl.DateTimeFormat('en-GB', { month: 'long', year: 'numeric' });
const weekdayShort = new Intl.DateTimeFormat('en-GB', { weekday: 'short' });
const dayLong = new Intl.DateTimeFormat('en-GB', { weekday: 'long', day: 'numeric', month: 'long' });
const dayShort = new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short' });
const time = new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit', hour12: false });

/** "November 2026". */
export const formatMonthTitle = (d: Date) => monthTitle.format(d);
/** "Mon". */
export const formatWeekday = (d: Date) => weekdayShort.format(d);
/** "Sunday 22 November". */
export const formatDayLong = (d: Date) => dayLong.format(d);
/** "16 – 22 Nov 2026" (or across months "30 Nov – 6 Dec 2026"). */
export function formatWeekRange(start: Date): string {
  const end = addDays(start, 6);
  const left = start.getMonth() === end.getMonth() ? String(start.getDate()) : dayShort.format(start);
  return `${left} – ${dayShort.format(end)} ${end.getFullYear()}`;
}
/** "21:00". */
export const formatTimeOfDay = (d: Date) => time.format(d);
