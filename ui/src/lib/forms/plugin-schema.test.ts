import { describe, expect, it } from 'vitest';
import {
  UnsupportedSchema, configSnippet, formatGoDuration, fromFormValues, parseGoDuration, permissionDiff, problemErrors,
  instanceMeta, sectionFor, toFormSchema, toFormValues,
} from './plugin-schema';
import type { ModuleField, ModulePermissions, ModuleSchema } from './plugin-types';

const every: ModuleSchema = {
  fields: [
    { key: 'url', type: 'string', label: 'Address', pattern: 'url-http', required: true, description: 'Where the client listens.' },
    { key: 'host', type: 'string', label: 'Host', pattern: 'hostname' },
    { key: 'folder', type: 'string', label: 'Folder', pattern: 'path-absolute' },
    { key: 'category', type: 'string', label: 'Category', pattern: 'identifier' },
    { key: 'note', type: 'string', label: 'Note', maxLength: 40 },
    { key: 'apiKey', type: 'secret', label: 'API key', required: true },
    { key: 'paused', type: 'bool', label: 'Add paused', default: false },
    { key: 'slots', type: 'int', label: 'Slots', min: 1, max: 10, step: 1, default: 4 },
    { key: 'ratio', type: 'number', label: 'Ratio', min: 0, max: 5, decimals: 2 },
    { key: 'priority', type: 'enum', label: 'Priority', options: [{ value: 'low', label: 'Low 🐢' }, { value: 'high', label: 'High ⚡' }] },
    { key: 'timeout', type: 'duration', label: 'Timeout', minDuration: '5s', maxDuration: '5m', default: '30s' },
    { key: 'tags', type: 'list<string>', label: 'Tags', maxItems: 5 },
    { key: 'ports', type: 'list<int>', label: 'Ports', maxItems: 3 },
    { key: 'paths', type: 'map<string,string>', label: 'Path mappings', maxEntries: 10 },
  ],
  groups: [
    { title: 'Connection', description: 'How corsarr reaches it.', fields: ['host', 'url'] },
    { title: 'Downloads', fields: ['paused', 'slots'] },
  ],
  actions: [{ id: 'test', label: 'Test connection' }, { id: 'discover', label: 'Find categories' }],
};

describe('toFormSchema', () => {
  it('maps every type of the closed set onto a field molecule', () => {
    const { schema } = toFormSchema(every);
    const byKey = Object.fromEntries(schema.fields.map((f) => [f.key, f]));
    expect(byKey.url).toMatchObject({ type: 'url', mono: true, required: true, help: 'Where the client listens.', validation: { schemes: ['http', 'https'] } });
    expect(byKey.host).toMatchObject({ type: 'text', mono: true });
    expect(byKey.host.validation?.pattern).toBeTruthy();
    expect(byKey.folder).toMatchObject({ type: 'path', mono: true });
    expect(byKey.category.validation?.pattern).toBe('^[a-z][a-zA-Z0-9]{0,47}$');
    expect(byKey.note).toMatchObject({ type: 'text' });
    expect(byKey.apiKey).toMatchObject({ type: 'text', secret: true });
    expect(byKey.paused).toMatchObject({ type: 'bool', default: false });
    expect(byKey.slots).toMatchObject({ type: 'number', default: 4, validation: { min: 1, max: 10 } });
    expect(byKey.ratio).toMatchObject({ type: 'number', validation: { min: 0, max: 5 } });
    expect(byKey.priority).toMatchObject({ type: 'select', options: [{ value: 'low', label: 'Low 🐢' }, { value: 'high', label: 'High ⚡' }] });
    expect(byKey.timeout).toMatchObject({ type: 'duration', default: 30, validation: { min: 5, max: 300 } });
    expect(byKey.tags).toMatchObject({ type: 'list', validation: { maxItems: 5 } });
    expect(byKey.ports).toMatchObject({ type: 'list', mono: true, validation: { maxItems: 3 } });
    expect(byKey.paths).toMatchObject({ type: 'map', mono: true });
  });

  it('orders grouped fields as the group lists them and keeps loose fields first', () => {
    const { schema } = toFormSchema(every);
    expect(schema.groups).toEqual([
      { id: 'g0', label: 'Connection', help: 'How corsarr reaches it.' },
      { id: 'g1', label: 'Downloads', help: undefined },
    ]);
    const keys = schema.fields.map((f) => `${f.key}:${f.group ?? ''}`);
    expect(keys.slice(0, 2)).toEqual(['folder:', 'category:']);
    expect(keys.slice(-4)).toEqual(['host:g0', 'url:g0', 'paused:g1', 'slots:g1']);
  });

  it('returns the actions and the plugin hints', () => {
    const r = toFormSchema({ ...every, fields: [...every.fields, { key: 'user', type: 'string', label: 'User', hint: 'The web UI login.' }] });
    expect(r.actions).toEqual([{ id: 'test', label: 'Test connection' }, { id: 'discover', label: 'Find categories' }]);
    expect(r.hints).toEqual({ user: 'The web UI login.' });
  });

  it('refuses any type outside the closed set', () => {
    for (const type of ['html', 'markdown', 'url', 'list<bool>', 'map<string,int>', 'object', '']) {
      expect(() => toFormSchema({ fields: [{ key: 'x', type, label: 'X' }] })).toThrow(UnsupportedSchema);
    }
  });

  it('refuses unknown patterns, bad keys, unknown group fields and oversized schemas', () => {
    expect(() => toFormSchema({ fields: [{ key: 'x', type: 'string', label: 'X', pattern: '.*' }] })).toThrow(/pattern/);
    expect(() => toFormSchema({ fields: [{ key: 'X-y', type: 'string', label: 'X' }] })).toThrow(/key/);
    expect(() => toFormSchema({ fields: [{ key: 'a', type: 'bool', label: 'A' }], groups: [{ title: 'G', fields: ['b'] }] })).toThrow(/group/);
    expect(() => toFormSchema({ fields: [{ key: 'a', type: 'bool', label: 'A' }], actions: [1, 2, 3, 4].map((i) => ({ id: `a${i}`, label: 'Go' })) })).toThrow(/actions/);
    const many: ModuleField[] = Array.from({ length: 51 }, (_, i) => ({ key: `f${i}`, type: 'bool', label: 'F' }));
    expect(() => toFormSchema({ fields: many })).toThrow(/fields/);
    expect(() => toFormSchema(null as unknown as ModuleSchema)).toThrow(UnsupportedSchema);
  });
});

describe('values', () => {
  it('reads config values (also the env export strings) into form values', () => {
    const v = toFormValues(every.fields, { slots: '6', ratio: 1.5, paused: 'true', timeout: '1m30s', ports: [8080, '9090'], url: 'http://box:8080' });
    expect(v).toMatchObject({ slots: 6, ratio: 1.5, paused: true, timeout: 90, ports: ['8080', '9090'], url: 'http://box:8080' });
  });

  it('writes form values back in the module types and drops empty secrets', () => {
    const v = fromFormValues(every.fields, { timeout: 90, ports: ['8080', ' ', '9090'], apiKey: '', slots: null, tags: ['a', ''], paths: [{ key: '/dl', value: '/data' }], url: 'http://x' });
    expect(v).toEqual({ timeout: '1m30s', ports: [8080, 9090], tags: ['a'], paths: { '/dl': '/data' }, url: 'http://x' });
    expect(fromFormValues(every.fields, { apiKey: 'typed' })).toEqual({ apiKey: 'typed' });
  });

  it('formats and parses Go durations', () => {
    expect(formatGoDuration(0)).toBe('0s');
    expect(formatGoDuration(45)).toBe('45s');
    expect(formatGoDuration(3600 + 120)).toBe('1h2m');
    expect(parseGoDuration('1h2m3s')).toBe(3723);
    expect(parseGoDuration('1.5m')).toBe(90);
    expect(parseGoDuration('500ms')).toBe(0.5);
    expect(parseGoDuration('soon')).toBeNull();
  });
});

describe('permissionDiff', () => {
  it('lists added and removed hosts and secrets', () => {
    const d = permissionDiff(
      { http: { hosts: ['api.example.org', '$setting:url'] }, secrets: ['apiKey'] },
      { http: { hosts: ['$setting:url', 'cdn.example.org', '*.example.net'] }, secrets: ['apiKey', 'password'] },
    );
    expect(d).toEqual({ addedHosts: ['cdn.example.org', '*.example.net'], removedHosts: ['api.example.org'], addedSecrets: ['password'], removedSecrets: [] });
  });
  it('treats a missing old set as nothing approved', () => {
    expect(permissionDiff(undefined, { http: {} })).toEqual({ addedHosts: [], removedHosts: [], addedSecrets: [], removedSecrets: [] });
    expect(permissionDiff(undefined, { http: { hosts: ['a.example'] } }).addedHosts).toEqual(['a.example']);
  });
  it('reads a set without an http part as no hosts', () => {
    // Permissions decoded from a hand-written manifest may leave the http part out.
    const noHttp = (p: object) => p as ModulePermissions;
    expect(permissionDiff(noHttp({ secrets: ['k'] }), noHttp({ secrets: ['k'] }))).toEqual({ addedHosts: [], removedHosts: [], addedSecrets: [], removedSecrets: [] });
    expect(permissionDiff({ http: { hosts: ['a.example'] } }, noHttp({})).removedHosts).toEqual(['a.example']);
  });
});

describe('configSnippet', () => {
  it('writes a new instance under its section with secrets as references', () => {
    const yaml = configSnippet({
      section: 'downloadClients', key: 'box', plugin: 'harbourdl', name: 'Box: upstairs', fields: every.fields,
      values: { url: 'http://box:8080', apiKey: 'sk-typed-value', paused: true, slots: 4, timeout: '1m', tags: ['tv', 'yes'], paths: { '/downloads': '/data' }, note: '' },
    });
    expect(yaml).toBe([
      'downloadClients:',
      '  box:',
      '    type: harbourdl',
      '    name: "Box: upstairs"',
      '    url: http://box:8080',
      '    apiKey: ${env:CORSARR_SECRET_DOWNLOADCLIENTS_BOX_APIKEY}',
      '    paused: true',
      '    timeout: 1m',
      '    tags:',
      '      - tv',
      '      - "yes"',
      '    paths:',
      '      /downloads: /data',
      '',
    ].join('\n'));
    expect(yaml).not.toContain('sk-typed-value');
  });

  it('quotes anything YAML would read as another type or as syntax', () => {
    const yaml = configSnippet({ section: 'downloadClients', key: 'x', plugin: 'p', fields: [{ key: 'note', type: 'string', label: 'N' }], values: { note: '#1 "a" <b>\nnext' } });
    expect(yaml).toContain('    note: "#1 \\"a\\" <b>\\nnext"');
  });
});

describe('helpers', () => {
  it('maps a module kind to its config section', () => {
    expect(sectionFor('downloader')).toBe('downloadClients');
    expect(sectionFor('indexer')).toBe('indexers');
    expect(sectionFor('something')).toBe('something');
  });

  it('turns server problems into field messages with the plugin hint', () => {
    const r = problemErrors([
      { path: 'values.url', message: 'must be an http or https address' },
      { path: 'tags[2]', msg: 'too long' },
      { path: 'other', message: 'unknown setting' },
    ], ['url', 'tags'], { url: 'Include the port.' });
    expect(r.fields).toEqual({ url: 'must be an http or https address. Include the port.', tags: 'too long' });
    expect(r.other).toEqual(['other: unknown setting']);
  });
});

describe('toFormSchema edge cases', () => {
  const one = (f: Partial<ModuleField> & { type: string }) => toFormSchema({ fields: [{ key: 'x', label: 'X', ...f }] }).schema.fields[0];

  it('labels a field by its key when it has no label and leaves optional fields unrequired', () => {
    const f = toFormSchema({ fields: [{ key: 'host', type: 'string' } as ModuleField] }).schema.fields[0];
    expect(f).toEqual({ key: 'host', type: 'text', label: 'host', help: undefined, required: undefined });
    expect(one({ type: 'string', pattern: '' })).toMatchObject({ type: 'text' });
    expect(one({ type: 'string', pattern: '' }).mono).toBeUndefined();
  });

  it('never carries a default into a secret', () => {
    expect(one({ type: 'secret', default: 'hunter2' }).default).toBeUndefined();
  });

  it('keeps a default and leaves bounds out when there are none', () => {
    expect(one({ type: 'int', default: 3 })).toEqual({ key: 'x', type: 'number', label: 'X', help: undefined, required: undefined, default: 3 });
    expect(one({ type: 'int', min: 2 }).validation).toEqual({ min: 2, max: undefined });
    expect(one({ type: 'enum' }).options).toEqual([]);
  });

  it('reads Go durations for the default and bounds and ignores what is not one', () => {
    expect(one({ type: 'duration' })).toMatchObject({ type: 'duration', default: undefined });
    expect(one({ type: 'duration' }).validation).toBeUndefined();
    expect(one({ type: 'duration', default: 45, maxDuration: 'later', minDuration: '1m' })).toMatchObject({ default: undefined, validation: { min: 60, max: undefined } });
    expect(one({ type: 'duration', default: 'soon' }).default).toBeUndefined();
  });

  it('turns an int list default into strings and leaves item limits out when unset', () => {
    expect(one({ type: 'list<int>', default: [80, 443] })).toMatchObject({ type: 'list', mono: true, default: ['80', '443'], validation: undefined });
    expect(one({ type: 'list<int>', default: 80 }).default).toBeUndefined();
    expect(one({ type: 'list<string>' }).validation).toBeUndefined();
  });

  it('leaves groups out when the schema has none', () => {
    const r = toFormSchema({ fields: [{ key: 'a', type: 'bool', label: 'A' }] });
    expect(r.schema.groups).toBeUndefined();
    expect(r.actions).toEqual([]);
    expect(r.hints).toEqual({});
  });

  it('accepts a group with no field list', () => {
    const r = toFormSchema({ fields: [{ key: 'a', type: 'bool', label: 'A' }], groups: [{ title: 'Empty' } as never] });
    expect(r.schema.groups).toEqual([{ id: 'g0', label: 'Empty', help: undefined }]);
    expect(r.schema.fields.map((f) => f.key)).toEqual(['a']);
  });

  it('refuses non-object fields, shared keys, repeated group fields, bad action ids and too many groups', () => {
    expect(() => toFormSchema({ fields: [null as unknown as ModuleField] })).toThrow(/not an object/);
    expect(() => toFormSchema({ fields: ['x' as unknown as ModuleField] })).toThrow(/not an object/);
    expect(() => toFormSchema({ fields: [{ key: 5, type: 'bool', label: 'A' } as unknown as ModuleField] })).toThrow(/key/);
    const a: ModuleField = { key: 'a', type: 'bool', label: 'A' };
    expect(() => toFormSchema({ fields: [a, a] })).toThrow(/share a key/);
    expect(() => toFormSchema({ fields: [a], groups: [{ title: 'G', fields: ['a'] }, { title: 'H', fields: ['a'] }] })).toThrow(/group 2/);
    expect(() => toFormSchema({ fields: [a], actions: [{ id: 'Bad id', label: 'Go' }] })).toThrow(/action id/);
    expect(() => toFormSchema({ fields: [a], groups: Array.from({ length: 11 }, () => ({ title: 'G', fields: [] })) })).toThrow(/groups/);
    expect(() => toFormSchema({ fields: {} as never })).toThrow(/field list/);
  });

  it('accepts exactly the most fields, groups and actions it allows', () => {
    const fields: ModuleField[] = Array.from({ length: 50 }, (_, i) => ({ key: `f${i}`, type: 'bool', label: `F${i}` }));
    const groups = Array.from({ length: 10 }, (_, i) => ({ title: `G${i}`, fields: [`f${i}`] }));
    const actions = Array.from({ length: 3 }, (_, i) => ({ id: `a${i}`, label: `A${i}` }));
    const r = toFormSchema({ fields, groups, actions });
    expect([r.schema.fields.length, r.schema.groups?.length, r.actions.length]).toEqual([50, 10, 3]);
    expect(() => toFormSchema({ fields: [...fields, { key: 'extra', type: 'bool', label: 'X' }] })).toThrow(/more than 50 fields/);
    expect(() => toFormSchema({ fields, actions: [...actions, { id: 'more', label: 'M' }] })).toThrow(/more than 3 actions/);
  });

  it('keeps a zero duration default', () => {
    const r = toFormSchema({ fields: [{ key: 'wait', type: 'duration', label: 'Wait', default: '0s' }] });
    expect(r.schema.fields[0].default).toBe(0);
  });
});

describe('instanceMeta', () => {
  it('shows a secret as set by its reference, and locks other fields even with a reference of the same name', () => {
    const fields: ModuleField[] = [{ key: 'apiKey', type: 'secret', label: 'K' }, { key: 'url', type: 'string', label: 'U' }, { key: 'token', type: 'secret', label: 'T' }];
    expect(instanceMeta(fields, 'downloadClients', 'up', { apikey: 'REF_A', url: 'REF_U' })).toEqual({
      apiKey: { managed: 'downloadClients.up.apiKey', isSet: true, ref: '${env:REF_A}' },
      url: { managed: 'downloadClients.up.url' },
      token: { managed: 'downloadClients.up.token' },
    });
  });
});

describe('value edge cases', () => {
  const fields: ModuleField[] = [
    { key: 'apiKey', type: 'secret', label: 'K' },
    { key: 'on', type: 'bool', label: 'On' },
    { key: 'wait', type: 'duration', label: 'Wait' },
    { key: 'tags', type: 'list<string>', label: 'Tags' },
    { key: 'paths', type: 'map<string,string>', label: 'Paths' },
    { key: 'mode', type: 'enum', label: 'Mode' },
    { key: 'name', type: 'string', label: 'Name' },
  ];

  it('reads values, skipping secrets and missing ones', () => {
    expect(toFormValues(fields, { apiKey: 'leak', on: 'false', wait: 30, tags: 'one', paths: { a: 'b' }, mode: 'fast', name: null })).toEqual({
      on: false, wait: 30, tags: ['one'], paths: { a: 'b' }, mode: 'fast',
    });
    expect(toFormValues(fields, { on: true, tags: ['a', 2] })).toEqual({ on: true, tags: ['a', '2'] });
    expect(toFormValues(fields, { wait: 'nonsense' })).toEqual({ wait: null });
  });

  it('writes values, passing through a map that is already an object and skipping empty ones', () => {
    expect(fromFormValues(fields, { paths: { a: 'b' }, mode: 'fast', on: false, name: undefined, wait: null, tags: [] })).toEqual({
      paths: { a: 'b' }, mode: 'fast', on: false, tags: [],
    });
    expect(fromFormValues(fields, { paths: [{ key: ' a ', value: '1' }, { key: ' ', value: 'x' }] })).toEqual({ paths: { a: '1' } });
  });
});

describe('configSnippet edge cases', () => {
  const base = { section: 'indexers', key: 'one', plugin: 'torznab' };

  it('writes a reference for a required secret even when nothing was typed and skips an optional one', () => {
    const yaml = configSnippet({ ...base, fields: [
      { key: 'apiKey', type: 'secret', label: 'K', required: true },
      { key: 'cookie', type: 'secret', label: 'C' },
      { key: 'token', type: 'secret', label: 'T' },
    ], values: { cookie: '', token: 'typed' } });
    expect(yaml).toBe([
      'indexers:', '  one:', '    type: torznab',
      '    apiKey: ${env:CORSARR_SECRET_INDEXERS_ONE_APIKEY}',
      '    token: ${env:CORSARR_SECRET_INDEXERS_ONE_TOKEN}', '',
    ].join('\n'));
  });

  it('leaves out defaults, empty lists, empty maps and missing values; keeps env references as they are', () => {
    const yaml = configSnippet({ ...base, fields: [
      { key: 'slots', type: 'int', label: 'S', default: 4 },
      { key: 'tags', type: 'list<string>', label: 'T' },
      { key: 'paths', type: 'map<string,string>', label: 'P' },
      { key: 'gone', type: 'string', label: 'G' },
      { key: 'url', type: 'string', label: 'U' },
      { key: 'ratio', type: 'number', label: 'R', default: 1 },
      { key: 'host', type: 'string', label: 'H' },
    ], values: { slots: 4, tags: [], paths: {}, gone: null, url: '${env:INDEXER_URL}', ratio: 2, host: 'true' } });
    expect(yaml).toBe(['indexers:', '  one:', '    type: torznab', '    url: ${env:INDEXER_URL}', '    ratio: 2', '    host: "true"', ''].join('\n'));
  });
});
