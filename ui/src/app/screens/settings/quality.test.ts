import { describe, it, expect } from 'vitest';
import type { ConditionKindInfo, ProfileRow, QualityChoice } from '../../api/types';
import {
  conditionSummary, cutoffOptions, formatSummary, groupsFromItems, kindsLabel, ladderItems, profileDoc, refusal, repairCutoff,
} from './quality';

const ladder: QualityChoice[] = [
  { id: 'unknown', name: 'Unknown', belowFloor: true },
  { id: 'webdl-720p', name: 'WEBDL-720p' },
  { id: 'webrip-1080p', name: 'WEBRip-1080p' },
  { id: 'webdl-1080p', name: 'WEBDL-1080p' },
  { id: 'bluray-1080p', name: 'Bluray-1080p' },
];
const kinds: ConditionKindInfo[] = [
  { kind: 'title', label: 'Release title', input: 'pattern' },
  { kind: 'audio', label: 'Audio codec', input: 'values', values: [{ value: 'aac', label: 'AAC' }, { value: 'ac3', label: 'AC3' }] },
  { kind: 'size', label: 'Size', input: 'range', unit: 'GB' },
];

describe('profile ladder', () => {
  const groups = [{ qualities: ['webdl-720p'] }, { name: 'WEB 1080p', qualities: ['webrip-1080p', 'webdl-1080p'] }];

  it('shows allowed ranks best first, then the rest switched off with the floor locked', () => {
    const items = ladderItems(groups, ladder);
    expect(items.map((i) => i.label)).toEqual(['WEB 1080p', 'WEBDL-720p', 'Bluray-1080p', 'Unknown']);
    expect(items[0].children?.map((c) => c.id)).toEqual(['q:webrip-1080p', 'q:webdl-1080p']);
    expect(items[2].enabled).toBe(false);
    expect(items[3].locked).toMatch(/floor/);
  });

  it('round-trips and picks up switched-on rows', () => {
    const items = ladderItems(groups, ladder);
    expect(groupsFromItems(items)).toEqual(groups);
    items[2] = { ...items[2], enabled: true };
    const top = [items[2], ...items.filter((_, i) => i !== 2)];
    expect(groupsFromItems(top).at(-1)).toEqual({ qualities: ['bluray-1080p'] });
  });

  it('offers allowed cutoffs best first and repairs a lost cutoff', () => {
    expect(cutoffOptions(groups, ladder).map((o) => o.label)).toEqual(['WEBRip-1080p (WEB 1080p)', 'WEBDL-1080p (WEB 1080p)', 'WEBDL-720p']);
    expect(repairCutoff('bluray-1080p', groups)).toBe('webdl-1080p');
    expect(repairCutoff('webdl-720p', groups)).toBe('webdl-720p');
    expect(repairCutoff('x', [])).toBe('');
  });
});

describe('summaries and refusals', () => {
  it('summarises conditions and formats', () => {
    expect(conditionSummary({ kind: 'audio', values: ['aac', 'ac3'] }, kinds)).toBe('Audio codec is AAC or AC3');
    expect(conditionSummary({ kind: 'title', pattern: '\\bDTS\\b', negate: true }, kinds)).toBe('Release title does not match \\bDTS\\b');
    expect(conditionSummary({ kind: 'size', min: 1, max: 10 }, kinds)).toBe('Size is 1–10 GB');
    expect(conditionSummary({ kind: 'size', min: 5 }, kinds)).toBe('Size is above 5 GB');
    expect(formatSummary({ key: 'f', name: 'F', conditions: [{ kind: 'title', pattern: 'a' }, { kind: 'title', pattern: 'b' }, { kind: 'title', pattern: 'c' }] }, kinds))
      .toBe('Release title does match a; Release title does match b (+1 more)');
    expect(kindsLabel(['tv', 'movie'])).toBe('TV and movies');
  });

  it('routes server errors to conditions and fields', () => {
    const r = refusal({ fields: [{ field: 'conditions.1.pattern', message: 'bad' }, { field: 'groups.0.qualities.1', message: 'twice' }, { field: 'name', message: 'empty' }] });
    expect(r.conditions[1]).toEqual({ pattern: 'bad' });
    expect(r.fields).toEqual({ groups: 'twice', name: 'empty' });
  });

  it('strips read-only row facts from a profile', () => {
    const row = { key: 'p', name: 'P', kinds: ['tv'], groups: [], cutoff: '', upgrades: true, minFormatScore: 0, cutoffFormatScore: 0,
      builtin: false, origin: 'database', items: 2, isDefault: false } as unknown as ProfileRow;
    expect(Object.keys(profileDoc(row)).sort()).toEqual(['cutoff', 'cutoffFormatScore', 'groups', 'key', 'kinds', 'minFormatScore', 'name', 'upgrades']);
  });
});

describe('ladder rows by group shape', () => {
  it('names a lone quality by itself and an unknown quality by its id', () => {
    const items = ladderItems([{ qualities: ['webdl-720p'] }, { qualities: ['remux-2160p'] }], ladder);
    expect(items.slice(0, 2)).toEqual([
      { id: 'q:remux-2160p', label: 'remux-2160p', enabled: true },
      { id: 'q:webdl-720p', label: 'WEBDL-720p', enabled: true },
    ]);
  });

  it('keeps a named group of one as a group, and joins the names of an unnamed group', () => {
    const items = ladderItems([{ name: 'HD', qualities: ['webdl-720p'] }, { qualities: ['webdl-1080p', 'remux-2160p'] }], ladder);
    expect(items[0]).toMatchObject({ id: 'g:0:', label: 'WEBDL-1080p + remux-2160p' });
    expect(items[0].children?.map((c) => c.label)).toEqual(['WEBDL-1080p', 'remux-2160p']);
    expect(items[1]).toMatchObject({ id: 'g:1:HD', label: 'HD' });
    expect(items[1].children?.map((c) => c.id)).toEqual(['q:webdl-720p']);
  });

  it('switches every quality off, locking only those below the floor, when nothing is allowed', () => {
    const items = ladderItems([], ladder);
    expect(items.map((i) => i.enabled)).toEqual([false, false, false, false, false]);
    expect(items.filter((i) => i.locked).map((i) => i.label)).toEqual(['Unknown']);
  });
});

describe('condition summaries', () => {
  const sum = (c: Parameters<typeof conditionSummary>[0]) => conditionSummary(c, kinds);

  it('words patterns, with or without a pattern', () => {
    expect(sum({ kind: 'title', pattern: 'x264' })).toBe('Release title does match x264');
    expect(sum({ kind: 'title' })).toBe('Release title does match ');
  });

  it('words ranges with and without a unit, a minimum and negation', () => {
    expect(sum({ kind: 'size', max: 4 })).toBe('Size is 0–4 GB');
    expect(sum({ kind: 'size' })).toBe('Size is above 0 GB');
    expect(sum({ kind: 'size', min: 1, max: 2, negate: true })).toBe('Size is not 1–2 GB');
    const unitless: ConditionKindInfo[] = [{ kind: 'year', label: 'Year', input: 'range' }];
    expect(conditionSummary({ kind: 'year', min: 1990, max: 2000 }, unitless)).toBe('Year is 1990–2000');
  });

  it('words value lists of every length, unknown values, negation and exceptions', () => {
    expect(sum({ kind: 'audio', values: ['aac'] })).toBe('Audio codec is AAC');
    expect(sum({ kind: 'audio', values: ['aac', 'flac', 'ac3'] })).toBe('Audio codec is AAC, flac or AC3');
    expect(sum({ kind: 'audio' })).toBe('Audio codec is ');
    expect(sum({ kind: 'audio', values: ['aac'], negate: true })).toBe('Audio codec is not AAC');
    expect(sum({ kind: 'audio', values: ['aac', 'ac3'], except: true })).toBe('Audio codec is anything but AAC or AC3');
  });

  it('falls back to the raw kind and values when the kind is not offered', () => {
    expect(sum({ kind: 'codec', values: ['x265'] })).toBe('codec is x265');
    const noValues: ConditionKindInfo[] = [{ kind: 'codec', label: 'Video codec', input: 'values' }];
    expect(conditionSummary({ kind: 'codec', values: ['x265'] }, noValues)).toBe('Video codec is x265');
  });
});

describe('refusal routing', () => {
  it('keeps the message and no fields when the body carries none', () => {
    expect(refusal(undefined, 'Refused.')).toEqual({ fields: {}, conditions: [], message: 'Refused.' });
    expect(refusal({})).toEqual({ fields: {}, conditions: [], message: undefined });
  });

  it('merges messages for one condition and joins messages for one field', () => {
    const r = refusal({ fields: [
      { field: 'conditions.0.pattern', message: 'bad' }, { field: 'conditions.0.kind', message: 'unknown' },
      { field: 'groups.0', message: 'empty.' }, { field: 'groups.1.qualities', message: 'twice.' },
    ] });
    expect(r.conditions[0]).toEqual({ pattern: 'bad', kind: 'unknown' });
    expect(r.fields).toEqual({ groups: 'empty. twice.' });
  });

  it('puts messages without a field on the form', () => {
    const r = refusal({ fields: [{ message: 'Nothing to save.' }, { field: '', message: 'Try again.' }] });
    expect(r.fields).toEqual({ form: 'Nothing to save. Try again.' });
    expect(r.conditions).toEqual([]);
  });
});
