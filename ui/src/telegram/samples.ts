// Synthetic sample data for the Telegram widgets: stories, tests and the static build.
// Fixed `now` so renders are reproducible.
import type { QueueListWidgetData, VariantPickerWidgetData } from './types';

export const sampleNow = Date.parse('2026-11-22T14:05:00Z');

export const variantPickerSample: VariantPickerWidgetData = {
  subject: { title: 'Dockside', kind: 'movie', subtitle: '2026', current: '2160p WEB-DL, stalled at 81% with no peers for 2h' },
  title: 'Replace with lower resolution',
  cap: 'Below 2160p',
  variants: [
    { id: 'l1', title: 'Dockside.2026.1080p.WEB-DL.DDP5.1-HARBOUR', quality: 'WEB-DL 1080p', size: 6.2e9, score: 900, reason: '12.0 GB smaller, same group', indexer: 'Harbour Tracker' },
    { id: 'l2', title: 'Dockside.2026.1080p.BluRay.x264-QUAY', quality: 'Bluray 1080p', size: 9.8e9, score: 750, reason: 'Well seeded (1,240)', indexer: 'Dock Feeds' },
    { id: 'l3', title: 'Dockside.2026.720p.WEB.h264-CRANE', quality: 'WEB 720p', size: 2.1e9, score: -150, reason: 'Below cutoff; would upgrade later', indexer: 'Usenet Pier' },
  ],
  defaultId: 'l1',
  autoAcceptAt: sampleNow + 42_000,
  now: sampleNow,
  formAction: '/telegram/replace/d2',
};

export const queueListSample: QueueListWidgetData = {
  title: 'Queue',
  now: sampleNow,
  formAction: '/telegram/queue',
  downloads: [
    {
      id: 'd2', title: 'Dockside', kind: 'movie', subtitle: '2026 · 2160p WEB-DL', state: 'stalled', reason: 'no peers for 2h',
      progress: 0.81, size: 18.2e9, downloaded: 14.7e9, speed: 0, rule: { badge: 'stalled', at: sampleNow + 245_000, action: 'Removes' },
    },
    {
      id: 'd1', title: 'The Long Haul', kind: 'tv', subtitle: 'S02E03 · Customs', state: 'downloading',
      progress: 0.46, size: 4.7e9, downloaded: 2.16e9, speed: 12.4e6, eta: 205,
    },
    {
      id: 'd3', title: 'The Long Haul', kind: 'tv', subtitle: 'S01 · 13 episodes', state: 'import-blocked', reason: 'sample only',
      size: 21e9, rule: { badge: 'stuck-import', at: sampleNow + 3 * 3600_000, action: 'Retries', dryRun: true },
    },
    {
      id: 'd4', title: 'Harbour Lights', kind: 'music', subtitle: 'Album · 11 tracks · FLAC', state: 'queued', paused: true,
      progress: 0.1, size: 412e6, downloaded: 41e6,
    },
    { id: 'd6', title: '港の灯り', kind: 'music', subtitle: 'Album · 9 tracks', state: 'imported', size: 98e6 },
  ],
};
