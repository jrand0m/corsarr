import { describe, expect, it } from 'vitest';
import { toSchema, toSchemaField } from './plugin-form';

describe('toSchemaField', () => {
  it('copies the basics and maps the manifest type onto a field molecule', () => {
    expect(toSchemaField({ key: 'limit', type: 'number', label: 'Limit', help: 'Per run.', required: true, default: 5 })).toEqual({
      key: 'limit', type: 'number', label: 'Limit', help: 'Per run.', required: true, default: 5,
    });
    expect(toSchemaField({ key: 'm', type: 'enum', label: 'M' }).type).toBe('select');
    expect(toSchemaField({ key: 'odd', type: 'html' as never, label: 'Odd' }).type).toBe('text');
  });

  it('marks secret types and secret-flagged fields write-only', () => {
    expect(toSchemaField({ key: 't', type: 'secret', label: 'T' })).toMatchObject({ type: 'text', secret: true });
    expect(toSchemaField({ key: 'p', type: 'text', label: 'P', secret: true }).secret).toBe(true);
    expect(toSchemaField({ key: 'p', type: 'text', label: 'P' }).secret).toBeUndefined();
  });

  it('turns options and bounds into select options and validation', () => {
    expect(toSchemaField({ key: 'm', type: 'enum', label: 'M', options: ['a'] }).options).toEqual([{ value: 'a', label: 'a' }]);
    expect(toSchemaField({ key: 'n', type: 'number', label: 'N', min: 1 }).validation).toEqual({ min: 1, max: undefined });
    expect(toSchemaField({ key: 'n', type: 'number', label: 'N', max: 9 }).validation).toEqual({ min: undefined, max: 9 });
    expect(toSchemaField({ key: 'n', type: 'number', label: 'N' }).validation).toBeUndefined();
  });

  it('reads showIf as a condition with booleans and plain values', () => {
    expect(toSchemaField({ key: 'a', type: 'text', label: 'A', showIf: 'on=true' }).visibleWhen).toEqual({ field: 'on', equals: true });
    expect(toSchemaField({ key: 'a', type: 'text', label: 'A', showIf: 'on=false' }).visibleWhen).toEqual({ field: 'on', equals: false });
    expect(toSchemaField({ key: 'a', type: 'text', label: 'A', showIf: 'mode=fast' }).visibleWhen).toEqual({ field: 'mode', equals: 'fast' });
    expect(toSchemaField({ key: 'a', type: 'text', label: 'A' }).visibleWhen).toBeUndefined();
  });

  it('shows URLs, paths and regexes in a monospace font', () => {
    for (const type of ['url', 'path', 'regex'] as const) expect(toSchemaField({ key: 'a', type, label: 'A' }).mono).toBe(true);
    expect(toSchemaField({ key: 'a', type: 'text', label: 'A' }).mono).toBeUndefined();
  });
});

describe('toSchema', () => {
  it('puts the instance name first', () => {
    const s = toSchema({ id: 'p', kind: 'indexer', version: '1', apiVersion: 1, name: 'P', delivery: 'builtin', settings: [{ key: 'url', type: 'url', label: 'URL' }] });
    expect(s.fields.map((f) => f.key)).toEqual(['name', 'url']);
  });
});
