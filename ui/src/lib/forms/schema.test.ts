import { describe, it, expect } from 'vitest';
import { changes, dirtyKeys, fromFormValue, initialValues, isVisible, toFormValue, validate, validateField, type SchemaField } from './schema';
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

describe('toFormValue', () => {
  const f = (type: SchemaField['type'], o: Partial<SchemaField> = {}): SchemaField => ({ key: 'k', type, label: 'K', ...o });

  it('takes the default when the server sent nothing and the empty value when neither is there', () => {
    expect(toFormValue(f('number', { default: 4 }), undefined)).toBe(4);
    expect(toFormValue(f('number', { default: 4 }), 7)).toBe(7);
    expect(toFormValue(f('number', { default: 4 }), null)).toBeNull();
    expect(toFormValue(f('text'), undefined)).toBe('');
    expect(toFormValue(f('bool'), null)).toBe(false);
    expect(toFormValue(f('list'), undefined)).toEqual([]);
  });

  it('never shows a stored secret', () => {
    expect(toFormValue(f('text', { secret: true, default: 'x' }), undefined)).toBe('');
  });

  it('turns map objects into entries with string values and copies entry lists', () => {
    expect(toFormValue(f('map'), { a: 1, b: 'two' })).toEqual([{ key: 'a', value: '1' }, { key: 'b', value: 'two' }]);
    const entries = [{ key: 'a', value: '1' }];
    const copy = toFormValue(f('map'), entries) as typeof entries;
    expect(copy).toEqual(entries);
    expect(copy[0]).not.toBe(entries[0]);
  });

  it('copies lists, reads select values as strings and passes the rest through', () => {
    const list = ['a', 'b'];
    expect(toFormValue(f('list'), list)).toEqual(list);
    expect(toFormValue(f('list'), list)).not.toBe(list);
    expect(toFormValue(f('multiselect'), list)).toEqual(list);
    expect(toFormValue(f('multiselect'), list)).not.toBe(list);
    expect(toFormValue(f('select'), 5)).toBe('5');
    expect(toFormValue(f('duration'), 90)).toBe(90);
    expect(toFormValue(f('text'), 'hi')).toBe('hi');
  });
});

describe('validateField', () => {
  const f = (type: SchemaField['type'], o: Partial<SchemaField> = {}): SchemaField => ({ key: 'k', type, label: 'Port', ...o });

  it('asks for required values, naming a select as a choice', () => {
    expect(validateField(f('text', { required: true }), '')).toBe('Port is required');
    expect(validateField(f('list', { required: true }), [])).toBe('Port is required');
    expect(validateField(f('duration', { required: true }), 0)).toBe('Port is required');
    expect(validateField(f('select', { required: true, label: 'Category' }), '')).toBe('Choose a category');
  });

  it('accepts a required zero number, a required unchecked box and a stored secret', () => {
    expect(validateField(f('number', { required: true }), 0)).toBeUndefined();
    expect(validateField(f('bool', { required: true }), false)).toBeUndefined();
    expect(validateField(f('text', { required: true, secret: true }), '', { isSet: true })).toBeUndefined();
    expect(validateField(f('text', { required: true, secret: true }), '', { ref: '${env:X}' })).toBeUndefined();
    expect(validateField(f('text', { required: true, secret: true }), '', {})).toBe('Port is required');
  });

  it('skips managed fields and empty optional ones', () => {
    expect(validateField(f('text', { required: true }), '', { managed: 'indexers.a.port' })).toBeUndefined();
    expect(validateField(f('number', { validation: { min: 5 } }), null)).toBeUndefined();
    expect(validateField(f('text', { validation: { pattern: '^x$' } }), '')).toBeUndefined();
  });

  it('checks number, duration and size ranges', () => {
    const range = { validation: { min: 1, max: 10 } };
    expect(validateField(f('number', range), 0)).toBe('Use 1 or more');
    expect(validateField(f('number', range), 11)).toBe('Use 10 or less');
    expect(validateField(f('number', range), 5)).toBeUndefined();
    expect(validateField(f('duration', range), 20)).toBe('Use 10 or less');
    expect(validateField(f('size', range), 0.5)).toBe('Use 1 or more');
    expect(validateField(f('number'), 1e9)).toBeUndefined();
  });

  it('checks URL schemes, regex syntax and duplicate map keys', () => {
    expect(validateField(f('url', { validation: { schemes: ['http', 'https'] } }), 'ftp://box')).toMatch(/http/);
    expect(validateField(f('url'), 'http://box:8080')).toBeUndefined();
    expect(validateField(f('regex'), '(')).toMatch(/^This pattern does not compile: /);
    expect(validateField(f('regex'), 'a+')).toBeUndefined();
    expect(validateField(f('map'), [{ key: 'a', value: '1' }, { key: 'a', value: '2' }])).toBe('Each key can appear only once');
    expect(validateField(f('map'), [{ key: 'a', value: '1' }, { key: 'b', value: '2' }])).toBeUndefined();
  });

  it('limits how many items a list or multiselect holds', () => {
    expect(validateField(f('list', { validation: { maxItems: 2 } }), ['a', 'b', 'c'])).toBe('Use at most 2');
    expect(validateField(f('multiselect', { validation: { maxItems: 2 } }), ['a', 'b', 'c'])).toBe('Use at most 2');
    expect(validateField(f('list', { validation: { maxItems: 2 } }), ['a', 'b'])).toBeUndefined();
    expect(validateField(f('list'), ['a', 'b', 'c'])).toBeUndefined();
  });

  it('matches text against the pattern with its own message or a generic one', () => {
    const host = { validation: { pattern: '^[a-z]+$', patternMessage: 'Port must be letters' } };
    expect(validateField(f('text', host), 'ABC')).toBe('Port must be letters');
    expect(validateField(f('path', { validation: { pattern: '^/' } }), 'data')).toBe('Port is not in the expected form');
    expect(validateField(f('text', host), 'abc')).toBeUndefined();
    expect(validateField(f('text'), 'anything')).toBeUndefined();
  });
});
