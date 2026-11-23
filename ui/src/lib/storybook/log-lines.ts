// Deterministic synthetic log lines for LogViewer stories and tests.
import type { LogLevel, LogLine } from '../format/log';

const start = Date.parse('2026-11-22T13:00:00Z');
const samples: [LogLevel, string, string, LogLine['fields']?][] = [
  ['info', 'scheduler', 'Feed sync started', { indexers: 4 }],
  ['debug', 'indexer', 'Query sent', { indexer: 'nzbgeek', q: 'The Long Haul S02E04', ms: 212 }],
  ['info', 'indexer', 'Feed sync finished', { new: 37, ms: 1840 }],
  ['info', 'decision', 'Release accepted', { release: 'The.Long.Haul.S02E04.1080p.WEB-DL-HARBOUR', score: 75 }],
  ['info', 'download', 'Grab sent', { client: 'harbourbox', id: 'a41f' }],
  ['trace', 'sse', 'Event published', { type: 'download.progress' }],
  ['warn', 'download', 'No progress for 30 minutes', { id: 'a41f', peers: 3, done: '61%' }],
  ['info', 'import', 'Imported', { path: '/tv/Quay Lights/Season 01/S01E08.mkv' }],
  ['error', 'indexer', 'Request failed: 503 Service Unavailable', { indexer: 'drunkenslug', attempt: 2, backoff: '5m' }],
  ['debug', 'metadata', 'Refreshed series', { id: 'tvdb:381422', episodes: 24 }],
  ['warn', 'health', 'Disk below the free-space floor', { path: '/downloads', free: '41.2 GB', floor: '50 GB' }],
  ['info', 'http', 'GET /api/v1/queue 200', { ms: 14 }],
];

export function logLines(n: number, offset = 0): LogLine[] {
  return Array.from({ length: n }, (_, k) => {
    const i = k + offset;
    const [level, module, message, fields] = samples[(i * 7) % samples.length];
    return { seq: i, at: start + i * 2_300, level, module, message, fields };
  });
}
