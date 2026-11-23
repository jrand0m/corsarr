import { describe, it, expect } from 'vitest';
import { changes, dirtyKeys, fromFormValue, initialValues, isVisible, toFormValue, validate } from './schema';
import { everyFieldSchema, everyFieldValues, everyFieldMeta } from '../storybook/plugin-schema';

const field = (key: string) => everyFieldSchema.fields.find((f) => f.key === key)!;

describe('settings schema logic', () => {
  it('fills defaults, empties secrets and turns maps into ordered entries', () => {
    const v = initialValues(everyFieldSchema, { headers: { A: '1', B: '2' } });
    expect(v.name).toBe('Harbour box');
    expect(v.slots).toBe(3);
    expect(v.password).toBe('');
    expect(v.headers).toEqual([{ key: 'A', value: '1' }, { key: 'B', value: '2' }]);
    expect(v.trackers).toEqual([]);
  });

  it('round-trips map and list values to their saved shape', () => {
    expect(fromFormValue(field('headers'), [{ key: ' X ', value: 'y' }, { key: '', value: 'dropped' }])).toEqual({ X: 'y' });
    expect(fromFormValue(field('trackers'), ['a', ' ', 'b'])).toEqual(['a', 'b']);
    expect(toFormValue(field('apiKey'), 'leaked')).toBe('');
  });

  it('follows conditional visibility, including through a hidden parent', () => {
    const schema = { fields: [
      { key: 'a', type: 'bool' as const, label: 'A' },
      { key: 'b', type: 'select' as const, label: 'B', visibleWhen: { field: 'a', equals: true } },
      { key: 'c', type: 'text' as const, label: 'C', visibleWhen: { field: 'b', equals: ['x', 'y'] } },
      { key: 'd', type: 'text' as const, label: 'D', visibleWhen: { field: 'c', set: true } },
    ] };
    expect(isVisible(schema.fields[1], { a: false }, schema)).toBe(false);
    expect(isVisible(schema.fields[2], { a: true, b: 'y' }, schema)).toBe(true);
    expect(isVisible(schema.fields[2], { a: false, b: 'y' }, schema)).toBe(false);
    expect(isVisible(schema.fields[3], { a: true, b: 'x', c: 'set' }, schema)).toBe(true);
  });

  it('validates required, ranges, URLs, regexes and duplicate keys; skips hidden and managed fields', () => {
    const v = { ...initialValues(everyFieldSchema), url: 'ftp://box', slots: 40, ignore: '(', headers: [{ key: 'a', value: '1' }, { key: 'a', value: '2' }], pauseReason: '' };
    const e = validate(everyFieldSchema, v, {});
    expect(e.apiKey).toBe('API key is required');
    expect(e.category).toBe('Choose a category');
    expect(e.url).toMatch(/Use http:\/\/ or https:\/\//);
    expect(e.slots).toBe('Use 20 or less');
    expect(e.ignore).toMatch(/does not compile/);
    expect(e.headers).toBe('Each key can appear only once');
    expect(e.pauseReason).toBeUndefined();
    const managed = validate(everyFieldSchema, v, { url: { managed: true }, apiKey: { isSet: true } });
    expect(managed.url).toBeUndefined();
    expect(managed.apiKey).toBeUndefined();
  });

  it('tracks dirty keys and sends only changed, visible, unmanaged values', () => {
    const base = initialValues(everyFieldSchema, everyFieldValues);
    const next = { ...base, slots: 5, password: 'n3w', pauseReason: 'ignored while hidden', headers: [{ key: 'X', value: '1' }] };
    expect(dirtyKeys(everyFieldSchema, base, next, everyFieldMeta).sort()).toEqual(['headers', 'password', 'pauseReason', 'slots']);
    expect(changes(everyFieldSchema, base, next, everyFieldMeta)).toEqual({ slots: 5, password: 'n3w', headers: { X: '1' } });
    expect(dirtyKeys(everyFieldSchema, base, next, { slots: { managed: true } })).not.toContain('slots');
  });
});
