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
