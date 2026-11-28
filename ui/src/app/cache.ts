// The artwork cache section of System › Health: usage against the cap and what the last
// shrinking run did, in plain words.
import { formatBytes } from '../lib/format/quantity';
import { formatAbsolute } from '../lib/format/time';
import type { Property } from '../lib/components/molecules/PropertyList/PropertyList.svelte';
import type { CacheRun, CacheStatus } from './api/types';

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

/** "412 MB of 1 GB" */
export function usageText(s: CacheStatus): string {
  return `${formatBytes(s.usage)} of ${formatBytes(s.maxSize)}`;
}

/** What one shrinking run did: "Lowered quality of 120 images, freed 4.2 MB". */
export function runText(r: CacheRun): string {
  const parts = [
    r.reencoded && `lowered the quality of ${plural(r.reencoded, 'image')}`,
    r.resized && `made ${plural(r.resized, 'image')} smaller`,
    r.evicted && `removed ${plural(r.evicted, 'image')}`,
    r.metadataEvicted && `removed ${plural(r.metadataEvicted, 'stored lookup')}`,
  ].filter(Boolean) as string[];
  const what = parts.length ? parts.join(', ') : 'changed nothing';
  return `${what[0].toUpperCase()}${what.slice(1)}; freed ${formatBytes(r.bytesFreed)}`;
}

export function cacheProperties(s: CacheStatus): Property[] {
  return [
    { key: 'images', label: 'Artwork stored', value: `${plural(s.images, 'image')}${s.variants ? ` + ${plural(s.variants, 'smaller copy', 'smaller copies')}` : ''}` },
    { key: 'hit', label: 'Served from the cache', value: s.requests ? `${Math.round(s.hitRate * 100)}% of ${plural(s.requests, 'view')} since start` : 'No views since start' },
    { key: 'meta', label: 'Stored lookups', value: formatBytes(s.metadataBytes) },
    { key: 'shrink', label: 'Shrinks at', value: `${Math.round(s.highWater * 100)}% full, down to ${Math.round(s.lowWater * 100)}%` },
    { key: 'last', label: 'Last shrink', value: s.lastRun ? `${formatAbsolute(new Date(s.lastRun.at))}: ${runText(s.lastRun)}` : 'Not needed yet' },
    { key: 'dir', label: 'Folder', value: s.dir, mono: true, copy: true },
  ];
}
