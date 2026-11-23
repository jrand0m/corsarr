// Log lines: levels, filtering and the plain-text form used for download. Pure.
import { toDate } from './time';

export const logLevels = ['trace', 'debug', 'info', 'warn', 'error'] as const;
export type LogLevel = (typeof logLevels)[number];

export interface LogLine {
  seq: number;
  at: Date | string | number;
  level: LogLevel;
  module: string;
  message: string;
  fields?: Record<string, string | number | boolean>;
}

export interface LogFilter { minLevel: LogLevel; module: string; text: string }

export function matchesLog(line: LogLine, f: LogFilter): boolean {
  if (logLevels.indexOf(line.level) < logLevels.indexOf(f.minLevel)) return false;
  if (f.module && line.module !== f.module) return false;
  const q = f.text.trim().toLowerCase();
  if (!q) return true;
  if (line.message.toLowerCase().includes(q) || line.module.toLowerCase().includes(q)) return true;
  return Object.entries(line.fields ?? {}).some(([k, v]) => `${k}=${v}`.toLowerCase().includes(q));
}

const clock = new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
/** "14:05:09". */
export const formatLogTime = (at: LogLine['at']) => clock.format(toDate(at));

export const formatFields = (fields?: LogLine['fields']) =>
  Object.entries(fields ?? {}).map(([k, v]) => `${k}=${typeof v === 'string' && /\s/.test(v) ? JSON.stringify(v) : v}`).join(' ');

/** One line of the downloaded file: ISO time, level, module, message, fields. */
export function formatLogLine(line: LogLine): string {
  const f = formatFields(line.fields);
  return `${toDate(line.at).toISOString()} ${line.level.toUpperCase().padEnd(5)} [${line.module}] ${line.message}${f ? ` ${f}` : ''}`;
}
