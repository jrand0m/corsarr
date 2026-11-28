// Fixture data for the custom format and quality profile screens: the owner's device
// formats and tiered profiles, the editor choices and a test result.
import type { FormatRow, FormatTestResult, ProfileRow, QualityChoices } from '../../app/api/types';

const video = [
  ['unknown', 'Unknown', true], ['sdtv', 'SDTV'], ['webdl-720p', 'WEBDL-720p'], ['bluray-720p', 'Bluray-720p'],
  ['webrip-1080p', 'WEBRip-1080p'], ['webdl-1080p', 'WEBDL-1080p'], ['bluray-1080p', 'Bluray-1080p'], ['remux-1080p', 'Remux-1080p'],
  ['webrip-2160p', 'WEBRip-2160p'], ['webdl-2160p', 'WEBDL-2160p'], ['bluray-2160p', 'Bluray-2160p'],
] as const;
const audio = [['mp3-96', 'MP3-96', true], ['mp3-320', 'MP3-320'], ['flac', 'FLAC'], ['flac-24', 'FLAC 24bit']] as const;
const ladder = (rows: readonly (readonly [string, string, boolean?])[]) => rows.map(([id, name, belowFloor]) => ({ id, name, belowFloor }));

export const qualityChoices: QualityChoices = {
  conditionKinds: [
    { kind: 'title', label: 'Release title', input: 'pattern' },
    { kind: 'group', label: 'Release group', input: 'pattern' },
    { kind: 'resolution', label: 'Resolution', input: 'values', values: [{ value: '720', label: '720p' }, { value: '1080', label: '1080p' }, { value: '2160', label: '2160p' }] },
    { kind: 'codec', label: 'Video codec', input: 'values', values: [{ value: 'h264', label: 'H.264' }, { value: 'h265', label: 'H.265 (HEVC)' }, { value: 'av1', label: 'AV1' }] },
    { kind: 'audio', label: 'Audio codec', input: 'values', values: [{ value: 'aac', label: 'AAC' }, { value: 'ac3', label: 'AC3 (Dolby Digital)' },
      { value: 'eac3', label: 'EAC3 (Dolby Digital Plus)' }, { value: 'dts', label: 'DTS' }, { value: 'truehd', label: 'TrueHD' }] },
    { kind: 'hdr', label: 'HDR format', input: 'values', values: [{ value: 'hdr10', label: 'HDR10' }, { value: 'dv', label: 'Dolby Vision' }, { value: 'sdr', label: 'SDR (no HDR)' }] },
    { kind: 'dvfallback', label: 'Dolby Vision fallback', input: 'values', values: [{ value: 'none', label: 'Without HDR10 fallback (profile 5)' }, { value: 'hdr10', label: 'With an HDR10 base' }] },
    { kind: 'size', label: 'Size', input: 'range', unit: 'GB' },
  ],
  qualities: { tv: ladder(video), movie: ladder(video), music: ladder(audio) },
};

export const formatRows: FormatRow[] = [
  { key: 'ps5audio', name: 'Audio - PS5 Safe (AC3/EAC3/AAC)', origin: 'database', usedBy: ['highps5', 'mediumps5'],
    conditions: [{ kind: 'audio', values: ['aac', 'ac3', 'eac3'] }] },
  { key: 'ps5noaudio', name: 'Audio - PS5 Incompatible (DTS/TrueHD)', origin: 'database', usedBy: ['highps5'],
    conditions: [{ kind: 'audio', values: ['dts', 'truehd'] }] },
  { key: 'nodirectplay', name: 'Reject - Not Direct Play', origin: 'config', managed: { key: 'customFormats.nodirectplay', kind: 'file', file: '/config/corsarr.yml' }, usedBy: [],
    conditions: [{ kind: 'dvfallback', values: ['none'] }, { kind: 'title', pattern: '\\b(Hi10P|10bit)\\b.*\\bx264\\b' }] },
];

const base = { minFormatScore: 0, cutoffFormatScore: 0, minUpgradeFormatScore: 1, upgrades: true };
export const profileRows: ProfileRow[] = [
  { ...base, key: 'any', name: 'Any', kinds: ['tv', 'movie'], builtin: true, origin: 'builtin', items: 0, cutoff: 'webdl-1080p',
    groups: [{ qualities: ['webdl-720p'] }, { name: 'WEB 1080p', qualities: ['webrip-1080p', 'webdl-1080p'] }] },
  { ...base, key: 'hd1080p', name: 'HD-1080p', kinds: ['tv', 'movie'], builtin: true, origin: 'builtin', items: 0, cutoff: 'bluray-1080p', default: ['tv', 'movie'],
    groups: [{ name: 'WEB 1080p', qualities: ['webrip-1080p', 'webdl-1080p'] }, { qualities: ['bluray-1080p'] }] },
  { ...base, key: 'highps5', name: 'High ~8GB/h - PS5', kinds: ['tv', 'movie'], builtin: false, origin: 'database', items: 42, cutoff: 'bluray-1080p',
    minFormatScore: 0, cutoffFormatScore: 100,
    groups: [{ name: 'WEB 1080p', qualities: ['webrip-1080p', 'webdl-1080p'] }, { qualities: ['bluray-1080p'] }],
    formats: [{ format: 'ps5audio', score: 100 }, { format: 'ps5noaudio', score: -10000 }, { format: 'nodirectplay', score: 0, reject: true }],
    sizes: [{ quality: 'bluray-1080p', min: 40, preferred: 100, max: 140 }] },
  { ...base, key: 'universallow', name: 'Universal - Low', kinds: ['tv', 'movie'], builtin: false, origin: 'config', items: 3, cutoff: 'webdl-720p',
    managed: { key: 'qualityProfiles.universallow', kind: 'file', file: '/config/corsarr.yml' }, groups: [{ qualities: ['webdl-720p'] }] },
  { ...base, key: 'lossless', name: 'Lossless', kinds: ['music'], builtin: true, origin: 'builtin', items: 0, cutoff: 'flac',
    groups: [{ qualities: ['flac'] }, { qualities: ['flac-24'] }] },
];

export function formatTest(title: string): FormatTestResult {
  const audioHit = /\b(AAC|E?AC-?3|DDP?)/i.test(title);
  return { title, parsed: {}, formats: [{ key: 'draft', name: 'Draft', matched: audioHit, conditions: [audioHit] }] };
}
